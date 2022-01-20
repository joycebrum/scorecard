// Copyright 2021 Security Scorecard Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package checks

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"go.uber.org/zap/zapcore"

	"github.com/ossf/scorecard/v4/checker"
	"github.com/ossf/scorecard/v4/clients/githubrepo"
	"github.com/ossf/scorecard/v4/clients/localdir"
	scut "github.com/ossf/scorecard/v4/utests"
)

func TestBinaryArtifacts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		inputFolder string
		err         error
		expected    checker.CheckResult
	}{
		{
			name:        "Jar file",
			inputFolder: "file://testdata/binaryartifacts/jars",
			err:         nil,
			expected: checker.CheckResult{
				Score: 9,
				Pass:  true,
			},
		},
		{
			name:        "non binary file",
			inputFolder: "file://testdata/licensedir/withlicense",
			err:         nil,
			expected: checker.CheckResult{
				Score: 10,
				Pass:  true,
			},
		},
	}
	for _, tt := range tests {
		tt := tt // Re-initializing variable so it is not changed while executing the closure below
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger, err := githubrepo.NewLogger(zapcore.DebugLevel)
			if err != nil {
				t.Errorf("githubrepo.NewLogger: %v", err)
			}

			// nolint
			defer logger.Sync()

			ctrl := gomock.NewController(t)
			repo, err := localdir.MakeLocalDirRepo(tt.inputFolder)

			if !errors.Is(err, tt.err) {
				t.Errorf("MakeLocalDirRepo: %v, expected %v", err, tt.err)
			}

			ctx := context.Background()

			client := localdir.CreateLocalDirClient(ctx, logger)
			if err := client.InitRepo(repo); err != nil {
				t.Errorf("InitRepo: %v", err)
			}

			dl := scut.TestDetailLogger{}

			req := checker.CheckRequest{
				Ctx:        ctx,
				RepoClient: client,
				Dlogger:    &dl,
			}

			result := BinaryArtifacts(&req)
			if result.Score != tt.expected.Score {
				t.Errorf("BinaryArtifacts: %v, expected %v for tests %v", result.Score, tt.expected.Score, tt.name)
			}
			if result.Pass != tt.expected.Pass {
				t.Errorf("BinaryArtifacts: %v, expected %v for tests %v", result.Pass, tt.expected.Pass, tt.name)
			}

			ctrl.Finish()
		})
	}
}
