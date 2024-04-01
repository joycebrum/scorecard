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

/*
TODO
  - Detects the end of the existing envvars at the first line that does not declare an
    envvar. This can lead to weird insertion positions if there is a comment in the
    middle of the `env:` block.
  - Tried performing a "dumber" implementation than the Python script, with less
    "parsing" of the workflow. However, the location given by f.Offset isn't precise
    enough. It only marks the start of the `run:` command, not the line where the
    variable is actually used. Will therefore need to, at least, parse the `run`
    command to replace all the instances of the unsafe variable. This means we can
    have multiple identical remediations if the same variable is used multiple times
    in the same step... that's just life.
*/
package patch

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/ossf/scorecard/v4/checker"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

const (
	assumedIndent = 2
)

// Fixes the script injection identified by the finding and returns a unified diff
// users can apply (with `git apply` or `patch`) to fix the workflow themselves.
// Should an error occur, it is handled and an empty patch is returned.
func GeneratePatch(f checker.File, content string) string {
	patchedWorkflow, ok := patchWorkflow(f, content)
	if !ok {
		return ""
	}
	return getDiff(f.Path, content, patchedWorkflow)
}

// Returns a patched version of the workflow without the script injection finding.
func patchWorkflow(f checker.File, content string) (string, bool) {
	unsafeVar := strings.Trim(f.Snippet, " ")
	runCmdIndex := f.Offset - 1

	lines := strings.Split(string(content), "\n")

	unsafePattern, ok := getUnsafePattern(unsafeVar)
	if !ok {
		return "", false
	}

	lines = replaceUnsafeVarWithEnvvar(lines, unsafePattern, runCmdIndex)
	lines, ok = addEnvvarsToGlobalEnv(lines, unsafePattern, unsafeVar)
	if !ok {
		return "", false
	}

	return strings.Join(lines, "\n"), true
}

// Adds a new global environment to a workflow. Assumes a global environment does not
// yet exist.
func addNewGlobalEnv(lines []string, globalIndentation int) ([]string, int, bool) {
	envPos, ok := findNewEnvPos(lines, globalIndentation)

	if !ok {
		// invalid workflow, could not determine location for new environment
		return nil, envPos, ok
	}

	label := strings.Repeat(" ", globalIndentation) + "env:"
	lines = slices.Insert(lines, envPos, []string{label, ""}...)
	return lines, envPos, ok
}

// Identifies the "global" indentation, as defined by the indentation on the required
// `on:` block. Will equal 0 in almost all cases.
func findGlobalIndentation(lines []string) (int, bool) {
	r := regexp.MustCompile(`^\s*on:`)
	for _, line := range lines {
		if r.MatchString(line) {
			return getIndent(line), true
		}
	}

	return -1, false
}

// Detects whether a global `env:` block already exists.
//
// Returns:
//   - int: the index for the line where the `env:` block is declared
//   - int: the indentation used for the declared environment variables
//   - bool: whether the `env` block exists
//
// The first two values return -1 if the `env` block doesn't exist
func findExistingEnv(lines []string, globalIndent int) (int, int, bool) {
	num_lines := len(lines)
	indent := strings.Repeat(" ", globalIndent)

	// regex to detect the global `env:` block
	labelRegex := regexp.MustCompile(indent + "env:")
	i := 0
	for i = 0; i < num_lines; i++ {
		line := lines[i]
		if labelRegex.MatchString(line) {
			break
		}
	}

	if i >= num_lines-1 {
		// there must be at least one more line
		return -1, -1, false
	}

	i++ // move to line after `env:`
	envvarIndent := getIndent(lines[i])
	// regex to detect envvars belonging to the global `env:` block
	envvarRegex := regexp.MustCompile(indent + `\s+[^#]`)
	for ; i < num_lines; i++ {
		line := lines[i]
		if !envvarRegex.MatchString(line) {
			// no longer declaring envvars
			break
		}
	}

	return i, envvarIndent, true
}

// Identifies the line where a new `env:` block should be inserted: right above the
// `jobs:` label.
//
// Returns:
//   - int: the index for the line where the `env:` block should be inserted
//   - bool: whether the `jobs:` block was found. Should always be `true`
func findNewEnvPos(lines []string, globalIndent int) (int, bool) {
	// the new env is added right before `jobs:`
	indent := strings.Repeat(" ", globalIndent)
	r := regexp.MustCompile(indent + "jobs:")
	for i, line := range lines {
		if r.MatchString(line) {
			return i, true
		}
	}

	return -1, false
}

type unsafePattern struct {
	envvarName   string
	idRegex      *regexp.Regexp
	replaceRegex *regexp.Regexp
}

func newUnsafePattern(e, p string) unsafePattern {
	return unsafePattern{
		envvarName:   e,
		idRegex:      regexp.MustCompile(p),
		replaceRegex: regexp.MustCompile(`{{\s*.*?` + p + `.*?\s*}}`),
	}
}

func getUnsafePattern(unsafeVar string) (unsafePattern, bool) {
	for _, p := range unsafePatterns {
		if p.idRegex.MatchString(unsafeVar) {
			p := p
			return p, true
		}
	}
	return unsafePattern{}, false
}

// Replaces all instances of the given script injection variable with the safe
// environment variable.
func replaceUnsafeVarWithEnvvar(lines []string, pattern unsafePattern, runIndex uint) []string {
	runIndent := getIndent(lines[runIndex])
	for i := int(runIndex); i < len(lines) && isParentLevelIndent(lines[i], runIndent); i++ {
		lines[i] = pattern.replaceRegex.ReplaceAllString(lines[i], pattern.envvarName)
	}

	return lines
}

// Adds the necessary environment variable to the global `env:` block, if it exists.
// If the `env:` block does not exist, it is created right above the `jobs:` label.
func addEnvvarsToGlobalEnv(lines []string, pattern unsafePattern, unsafeVar string) ([]string, bool) {
	globalIndentation, ok := findGlobalIndentation(lines)

	if !ok {
		// invalid workflow, could not determine global indentation
		return nil, false
	}

	envPos, envvarIndent, exists := findExistingEnv(lines, globalIndentation)

	if !exists {
		lines, envPos, ok = addNewGlobalEnv(lines, globalIndentation)
		if !ok {
			return nil, ok
		}

		// position now points to `env:`, insert variables below it
		envPos += 1
		envvarIndent = globalIndentation + assumedIndent
	}

	envvarDefinition := fmt.Sprintf("%s: ${{ %s }}", pattern.envvarName, unsafeVar)
	lines = slices.Insert(lines, envPos, strings.Repeat(" ", envvarIndent)+envvarDefinition)

	return lines, ok
}

var unsafePatterns = []unsafePattern{
	newUnsafePattern("AUTHOR_EMAIL", `github\.event\.commits.*?\.author\.email`),
	newUnsafePattern("AUTHOR_EMAIL", `github\.event\.head_commit\.author\.email`),
	newUnsafePattern("AUTHOR_NAME", `github\.event\.commits.*?\.author\.name`),
	newUnsafePattern("AUTHOR_NAME", `github\.event\.head_commit\.author\.name`),
	newUnsafePattern("COMMENT_BODY", `github\.event\.comment\.body`),
	newUnsafePattern("COMMIT_MESSAGE", `github\.event\.commits.*?\.message`),
	newUnsafePattern("COMMIT_MESSAGE", `github\.event\.head_commit\.message`),
	newUnsafePattern("ISSUE_BODY", `github\.event\.issue\.body`),
	newUnsafePattern("ISSUE_TITLE", `github\.event\.issue\.title`),
	newUnsafePattern("PAGE_NAME", `github\.event\.pages.*?\.page_name`),
	newUnsafePattern("PR_BODY", `github\.event\.pull_request\.body`),
	newUnsafePattern("PR_DEFAULT_BRANCH", `github\.event\.pull_request\.head\.repo\.default_branch`),
	newUnsafePattern("PR_HEAD_LABEL", `github\.event\.pull_request\.head\.label`),
	newUnsafePattern("PR_HEAD_REF", `github\.event\.pull_request\.head\.ref`),
	newUnsafePattern("PR_TITLE", `github\.event\.pull_request\.title`),
	newUnsafePattern("REVIEW_BODY", `github\.event\.review\.body`),
	newUnsafePattern("REVIEW_COMMENT_BODY", `github\.event\.review_comment\.body`),

	newUnsafePattern("HEAD_REF", `github\.head_ref`),
}

// Returns the indentation of the given line. The indentation is all whitespace and
// dashes before a key or value.
func getIndent(line string) int {
	return len(line) - len(strings.TrimLeft(line, " -"))
}

func isBlankOrComment(line string) bool {
	blank := regexp.MustCompile(`^\s*$`)
	comment := regexp.MustCompile(`^\s*#`)

	return blank.MatchString(line) || comment.MatchString(line)
}

// Returns true if the line's indentation matches its parent's indentation.
// Blank lines and pure comment lines are ignored (return false).
func isParentLevelIndent(line string, parentIndent int) bool {
	if isBlankOrComment(line) {
		return false
	}
	return getIndent(line) >= parentIndent
}

// Returns the changes between the original and patched workflows as a unified diff
// (the same generated by `git diff` or `diff -u`).
func getDiff(path, original, patched string) string {
	// initialize an in-memory repository
	repo := newInMemoryRepo()
	if repo == nil {
		return ""
	}

	// commit original workflow to in-memory repository
	originalCommit := commitWorkflow(path, original, repo)
	if originalCommit == nil {
		return ""
	}

	// commit patched workflow to in-memory repository
	patchedCommit := commitWorkflow(path, patched, repo)
	if patchedCommit == nil {
		return ""
	}

	return toUnifiedDiff(originalCommit, patchedCommit)
}

// Initializes an in-memory repository
func newInMemoryRepo() *git.Repository {
	// initialize an in-memory repository
	filesystem := memfs.New()
	repo, err := git.Init(memory.NewStorage(), filesystem)
	if err != nil {
		return nil
	}

	return repo
}

// Commits the workflow at the given path to the in-memory repository
func commitWorkflow(path, contents string, repo *git.Repository) *object.Commit {
	worktree, err := repo.Worktree()
	if err != nil {
		return nil
	}
	filesystem := worktree.Filesystem

	// create (or overwrite) file
	df, err := filesystem.Create(path)
	if err != nil {
		return nil
	}

	df.Write([]byte(contents))
	df.Close()

	// commit file to in-memory repository
	worktree.Add(path)
	hash, err := worktree.Commit("x", &git.CommitOptions{})
	if err != nil {
		return nil
	}

	commit, err := repo.CommitObject(hash)
	if err != nil {
		return nil
	}
	return commit
}

// Returns a unified diff describing the difference between the given commits
func toUnifiedDiff(originalCommit, patchedCommit *object.Commit) string {
	patch, err := originalCommit.Patch(patchedCommit)
	if err != nil {
		return ""
	}
	builder := strings.Builder{}
	patch.Encode(&builder)

	return builder.String()
}
