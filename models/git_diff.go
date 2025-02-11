// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Unknwon/com"

	"github.com/gogits/git"

	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/process"
)

// Diff line types.
const (
	DIFF_LINE_PLAIN = iota + 1
	DIFF_LINE_ADD
	DIFF_LINE_DEL
	DIFF_LINE_SECTION
)

const (
	DIFF_FILE_ADD = iota + 1
	DIFF_FILE_CHANGE
	DIFF_FILE_DEL
)

type DiffLine struct {
	LeftIdx  int
	RightIdx int
	Type     int
	Content  string
}

func (d DiffLine) GetType() int {
	return d.Type
}

type DiffSection struct {
	Name  string
	Lines []*DiffLine
}

type DiffFile struct {
	Name               string
	Index              int
	Addition, Deletion int
	Type               int
	IsBin              bool
	Sections           []*DiffSection
}

type Diff struct {
	TotalAddition, TotalDeletion int
	Files                        []*DiffFile
}

func (diff *Diff) NumFiles() int {
	return len(diff.Files)
}

const DIFF_HEAD = "diff --git "

func ParsePatch(pid int64, cmd *exec.Cmd, reader io.Reader) (*Diff, error) {
	scanner := bufio.NewScanner(reader)
	var (
		curFile    *DiffFile
		curSection = &DiffSection{
			Lines: make([]*DiffLine, 0, 10),
		}

		leftLine, rightLine int
	)

	diff := &Diff{Files: make([]*DiffFile, 0)}
	var i int
	for scanner.Scan() {
		line := scanner.Text()
		// fmt.Println(i, line)
		if strings.HasPrefix(line, "+++ ") || strings.HasPrefix(line, "--- ") {
			continue
		}

		i = i + 1

		// Diff data too large.
		if i == 5000 {
			log.Warn("Diff data too large")
			return &Diff{}, nil
		}

		if line == "" {
			continue
		}

		switch {
		case line[0] == ' ':
			diffLine := &DiffLine{Type: DIFF_LINE_PLAIN, Content: line, LeftIdx: leftLine, RightIdx: rightLine}
			leftLine++
			rightLine++
			curSection.Lines = append(curSection.Lines, diffLine)
			continue
		case line[0] == '@':
			curSection = &DiffSection{}
			curFile.Sections = append(curFile.Sections, curSection)
			ss := strings.Split(line, "@@")
			diffLine := &DiffLine{Type: DIFF_LINE_SECTION, Content: line}
			curSection.Lines = append(curSection.Lines, diffLine)

			// Parse line number.
			ranges := strings.Split(ss[len(ss)-2][1:], " ")
			leftLine, _ = com.StrTo(strings.Split(ranges[0], ",")[0][1:]).Int()
			rightLine, _ = com.StrTo(strings.Split(ranges[1], ",")[0]).Int()
			continue
		case line[0] == '+':
			curFile.Addition++
			diff.TotalAddition++
			diffLine := &DiffLine{Type: DIFF_LINE_ADD, Content: line, RightIdx: rightLine}
			rightLine++
			curSection.Lines = append(curSection.Lines, diffLine)
			continue
		case line[0] == '-':
			curFile.Deletion++
			diff.TotalDeletion++
			diffLine := &DiffLine{Type: DIFF_LINE_DEL, Content: line, LeftIdx: leftLine}
			if leftLine > 0 {
				leftLine++
			}
			curSection.Lines = append(curSection.Lines, diffLine)
		case strings.HasPrefix(line, "Binary"):
			curFile.IsBin = true
			continue
		}

		// Get new file.
		if strings.HasPrefix(line, DIFF_HEAD) {
			fs := strings.Split(line[len(DIFF_HEAD):], " ")
			a := fs[0]

			curFile = &DiffFile{
				Name:     a[strings.Index(a, "/")+1:],
				Index:    len(diff.Files) + 1,
				Type:     DIFF_FILE_CHANGE,
				Sections: make([]*DiffSection, 0, 10),
			}
			diff.Files = append(diff.Files, curFile)

			// Check file diff type.
			for scanner.Scan() {
				switch {
				case strings.HasPrefix(scanner.Text(), "new file"):
					curFile.Type = DIFF_FILE_ADD
				case strings.HasPrefix(scanner.Text(), "deleted"):
					curFile.Type = DIFF_FILE_DEL
				case strings.HasPrefix(scanner.Text(), "index"):
					curFile.Type = DIFF_FILE_CHANGE
				}
				if curFile.Type > 0 {
					break
				}
			}
		}
	}

	return diff, nil
}

func GetDiffRange(repoPath, beforeCommitId string, afterCommitId string) (*Diff, error) {
	repo, err := git.OpenRepository(repoPath)
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(afterCommitId)
	if err != nil {
		return nil, err
	}

	rd, wr := io.Pipe()
	var cmd *exec.Cmd
	// if "after" commit given
	if beforeCommitId == "" {
		// First commit of repository.
		if commit.ParentCount() == 0 {
			cmd = exec.Command("git", "show", afterCommitId)
		} else {
			c, _ := commit.Parent(0)
			cmd = exec.Command("git", "diff", c.Id.String(), afterCommitId)
		}
	} else {
		cmd = exec.Command("git", "diff", beforeCommitId, afterCommitId)
	}
	cmd.Dir = repoPath
	cmd.Stdout = wr
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	done := make(chan error)
	go func() {
		cmd.Start()
		done <- cmd.Wait()
		wr.Close()
	}()
	defer rd.Close()

	desc := fmt.Sprintf("GetDiffRange(%s)", repoPath)
	pid := process.Add(desc, cmd)
	go func() {
		// In case process became zombie.
		select {
		case <-time.After(5 * time.Minute):
			if errKill := process.Kill(pid); errKill != nil {
				log.Error(4, "git_diff.ParsePatch(Kill): %v", err)
			}
			<-done
			// return "", ErrExecTimeout.Error(), ErrExecTimeout
		case err = <-done:
			process.Remove(pid)
		}
	}()

	return ParsePatch(pid, cmd, rd)
}

func GetDiffCommit(repoPath, commitId string) (*Diff, error) {
	return GetDiffRange(repoPath, "", commitId)
}
