// Copyright (c) 2019 Target Brands, Inc. All rights reserved.
//
// Use of this source code is governed by the LICENSE file in this repository.

package linux

import (
	"context"
	"fmt"
	"time"

	"github.com/go-vela/sdk-go/vela"
	"github.com/go-vela/types/constants"
	"github.com/go-vela/types/pipeline"
	"github.com/sirupsen/logrus"
)

// CreateStage prepares the stage for execution.
func (c *client) CreateStage(ctx context.Context, s *pipeline.Stage) error {
	// update engine logger with extra metadata
	logger := c.logger.WithFields(logrus.Fields{
		"stage": s.Name,
	})

	// create the steps for the stage
	for _, step := range s.Steps {
		logger.Debugf("creating %s step", step.Name)
		// create the step
		err := c.CreateStep(ctx, step)
		if err != nil {
			return err
		}
	}

	return nil
}

// TODO: Make this do stuff
func (c *client) PlanStage(ctx context.Context, s *pipeline.Stage) error {
	return fmt.Errorf("this function is currently not supported")
}

// ExecStage runs a stage.
func (c *client) ExecStage(ctx context.Context, s *pipeline.Stage, m map[string]chan error) error {
	b := c.build
	r := c.repo

	// update engine logger with extra metadata
	logger := c.logger.WithFields(logrus.Fields{
		"stage": s.Name,
	})

	logger.Debug("gathering stage dependency tree")
	// ensure dependent stages have completed
	for _, needs := range s.Needs {
		logger.Debugf("looking up dependency %s", needs)
		// check if a dependency stage has completed
		stageErr, ok := m[needs]
		if !ok { // stage not found so we continue
			continue
		}

		logger.Debugf("waiting for dependency %s", needs)
		// wait for the stage channel to close
		select {
		case <-ctx.Done():
			return fmt.Errorf("errgroup context is done")
		case err := <-stageErr:
			if err != nil {
				logger.WithError(err).Errorf("%s stage produced error", needs)
				return err
			}
			continue
		}
	}

	// close the stage channel at the end
	defer close(m[s.Name])

	logger.Debug("starting execution of stage")
	// execute the steps for the stage
	for _, step := range s.Steps {
		c.logger.Infof("planning %s step", step.Name)
		// plan the step
		err := c.PlanStep(ctx, step)
		if err != nil {
			return fmt.Errorf("unable to plan step: %w", err)
		}

		logger.Debugf("executing %s step", step.Name)
		// execute the step
		err = c.ExecStep(ctx, step)
		if err != nil {
			return err
		}

		// check the step exit code
		if step.ExitCode != 0 {
			// check if we ignore step failures
			if !step.Ruleset.Continue {
				// set build status to failure
				b.Status = vela.String(constants.StatusFailure)
			}

			// update the step fields
			c.step.ExitCode = vela.Int(step.ExitCode)
			c.step.Status = vela.String(constants.StatusFailure)
		}

		c.step.Finished = vela.Int64(time.Now().UTC().Unix())
		c.logger.Infof("uploading %s step state", step.Name)
		// send API call to update the build
		_, _, err = c.Vela.Step.Update(r.GetOrg(), r.GetName(), b.GetNumber(), c.step)
		if err != nil {
			return err
		}
	}

	return nil
}

// DestroyStage cleans up the stage after execution.
func (c *client) DestroyStage(ctx context.Context, s *pipeline.Stage) error {
	// update logger with extra metadata
	logger := c.logger.WithFields(logrus.Fields{
		"stage": s.Name,
	})

	// destroy the steps for the stage
	for _, step := range s.Steps {
		logger.Debugf("destroying %s step", step.Name)
		// destroy the step
		err := c.DestroyStep(ctx, step)
		if err != nil {
			return err
		}
	}

	return nil
}