// Copyright 2024 OpenSSF Scorecard Authors
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

package patch

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/ossf/scorecard/v4/checker"
)

func Test_GeneratePatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		file     checker.File
		diffPath string
	}{
		// Extracted from real Angular fix: https://github.com/angular/angular/pull/51026/files
		{
			name: "Real Example 1",
			file: checker.File{
				Path:    "realExample1.yaml",
				Snippet: " github.event.comment.body ",
				Offset:  42,
			},
			diffPath: "realExample1.diff",
		},
		// // Inspired on a real fix: https://github.com/googleapis/google-cloud-go/pull/9011/files
		// {
		// 	name:             "Real Example 2",
		// 	inputFilepath:    "realExample2.yaml",
		// 	expectedFilepath: "realExample2_fixed.yaml",
		// },
		// // Inspired from a real lit/lit fix: https://github.com/lit/lit/pull/3669/files
		// {
		// 	name:             "Real Example 3",
		// 	inputFilepath:    "realExample3.yaml",
		// 	expectedFilepath: "realExample3_fixed.yaml",
		// },
		// {
		// 	name:             "Test all (or most) types of user input that should be detected",
		// 	inputFilepath:    "allKindsOfUserInput.yaml",
		// 	expectedFilepath: "allKindsOfUserInput_fixed.yaml",
		// },
		// {
		// 	name:             "User's input is assigned to a variable before used",
		// 	inputFilepath:    "userInputAssignedToVariable.yaml",
		// 	expectedFilepath: "userInputAssignedToVariable_fixed.yaml",
		// },
		// {
		// 	name:             "Two incidences in different jobs",
		// 	inputFilepath:    "twoInjectionsDifferentJobs.yaml",
		// 	expectedFilepath: "twoInjectionsDifferentJobs_fixed.yaml",
		// },
		// {
		// 	name:             "Two incidences in same job",
		// 	inputFilepath:    "twoInjectionsSameJob.yaml",
		// 	expectedFilepath: "twoInjectionsSameJob_fixed.yaml",
		// },
		// {
		// 	name:             "Two incidences in same step",
		// 	inputFilepath:    "twoInjectionsSameStep.yaml",
		// 	expectedFilepath: "twoInjectionsSameStep_fixed.yaml",
		// },
		// {
		// 	name:             "Reuse existent workflow level env var, if has the same name we'd give",
		// 	inputFilepath:    "reuseWorkflowLevelEnvVars.yaml",
		// 	expectedFilepath: "reuseWorkflowLevelEnvVars_fixed.yaml",
		// },
		// // Test currently failing because we don't look for existent env vars pointing to the same content.
		// // Once proper behavior is implemented, enable this test
		// // {
		// // 	name:             "Reuse existent workflow level env var, if it DOES NOT have the same name we'd give",
		// // 	inputFilepath:    "reuseEnvVarWithDiffName.yaml",
		// // 	expectedFilepath: "reuseEnvVarWithDiffName_fixed.yaml",
		// // },

		// // Test currently failing because we don't look for existent env vars on smaller scopes -- job-level or step-level.
		// // In this case, we're always creating a new workflow-level env var. Note that this could lead to creation of env vars shadowed
		// // by the ones in smaller scope.
		// // Once proper behavior is implemented, enable this test
		// // {
		// // 	name:             "Reuse env var already existent on smaller scope, it converts case of same or different names",
		// // 	inputFilepath:    "reuseEnvVarSmallerScope.yaml",
		// // 	expectedFilepath: "reuseEnvVarSmallerScope_fixed.yaml",
		// // },
		// {
		// 	name:             "4-spaces indentation is kept the same",
		// 	inputFilepath:    "fourSpacesIndentationExistentEnvVar.yaml",
		// 	expectedFilepath: "fourSpacesIndentationExistentEnvVar_fixed.yaml",
		// },
		// {
		// 	name:             "Crazy but valid indentation is kept the same",
		// 	inputFilepath:    "crazyButValidIndentation.yaml",
		// 	expectedFilepath: "crazyButValidIndentation_fixed.yaml",
		// },
		// {
		// 	name:             "Newline on EOF is kept",
		// 	inputFilepath:    "newlineOnEOF.yaml",
		// 	expectedFilepath: "newlineOnEOF_fixed.yaml",
		// },
		// // Test currently failing due to lack of style awareness. Currently we always add a blankline after
		// // the env block.
		// // Once proper behavior is implemented, enable this test.
		// // {
		// // 	name:             "Keep style if file doesnt use blank lines between blocks",
		// // 	inputFilepath:    "noLineBreaksBetweenBlocks.yaml",
		// // 	expectedFilepath: "noLineBreaksBetweenBlocks_fixed.yaml",
		// // },
		// {
		// 	name:             "Ignore if user input regex is just part of a comment",
		// 	inputFilepath:    "ignorePatternInsideComments.yaml",
		// 	expectedFilepath: "ignorePatternInsideComments.yaml",
		// },
	}
	for _, tt := range tests {
		tt := tt // Re-initializing variable so it is not changed while executing the closure below
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file := tt.file
			file.Path = path.Join("./testdata", file.Path)

			expectedDiff, err := parseDiffFile(tt.diffPath)
			if err != nil {
				t.Errorf("Couldn't read expected diff file. Error:\n%s", err)
			}

			inputContent, err := os.ReadFile(file.Path)
			if err != nil {
				t.Errorf("Couldn't read input testfile. Error:\n%s", err)
			}

			output := GeneratePatch(file, string(inputContent))
			if diff := cmp.Diff(expectedDiff, output); diff != "" {
				fmt.Println(output)
				fmt.Println(expectedDiff)
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// This function parses the diff file and makes a few changes necessary to make a
// valid comparison with the output of GeneratePatch.
//
// For example, the following diff file created with `git diff`:
//
//	diff --git a/testdata/foo.yaml b/testdata/foo_fixed.yaml
//	index 843d0c71..cced3454 100644
//	--- a/testdata/foo.yaml
//	+++ b/testdata/foo_fixed.yaml
//	@@ -6,6 +6,9 @@ jobs:
//	< ... the diff ... >
//
// becomes:
//
//	--- a/testdata/foo.yaml
//	+++ b/testdata/foo_fixed.yaml
//	@@ -6,6 +6,9 @@
//	< ... the diff ... >
//
// Note that, despite the differences between our output and the official
// `git diff`, our output is still valid and can be passed to
// `patch -p1 < path/to/file.diff` to apply the fix to the workflow.
func parseDiffFile(filepath string) (string, error) {
	c, err := os.ReadFile(path.Join("./testdata", filepath))
	if err != nil {
		return "", err
	}

	// The real `git diff` includes multiple "headers" (`diff --git ...`, `index ...`)
	// Our diff does not include these headers; it starts with the "in/out" headers of
	// --- a/path/to/file
	// +++ b/path/to/file
	// We must therefore remove any previous headers from the `git diff`.
	lines := strings.Split(string(c), "\n")
	i := 0
	var line string
	for i, line = range lines {
		if strings.HasPrefix(line, "--- ") {
			break
		}
	}
	content := strings.Join(lines[i:], "\n")

	// The real `git diff` adds contents after the `@@` anchors (the text of the line on
	// which the anchor is placed):
	// 		i.e. `@@ 1,2 3,4 @@ jobs:`
	// while ours does not
	//		i.e. `@@ 1,2 3,4 @@`
	// We must therefore remove that extra content to compare with our diff.
	r := regexp.MustCompile(`(@@[ \d,+-]+@@).*`)
	return r.ReplaceAllString(string(content), "$1"), nil
}
