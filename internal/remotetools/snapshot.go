package remotetools

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

const (
	maxSnapshotProcesses = 5000
	maxSnapshotCommand   = 1024
	maxSnapshotOutput    = 8 << 20
	snapshotReadRows     = maxSnapshotProcesses + 9
)

// ProcessSnapshot is a bounded host reading, not merely the busiest processes.
// The flags keep search scope honest at the limits.
type ProcessSnapshot struct {
	Processes         []Process
	Truncated         bool
	CommandsTruncated bool
}

func snapshotCommand(legacy bool) string {
	command := "LC_ALL=C " + collectorMarker + " -o pmem= -o etime= -o args= 2>&1"
	if legacy {
		command = legacyProcessCommand
	}
	return command + " | head -n " + strconv.Itoa(snapshotReadRows) + " | cut -c1-" + strconv.Itoa(maxSnapshotCommand+60)
}

// ReadProcessSnapshot never receives a search query: filtering the bounded
// result is local, so user input cannot become a remote shell command.
func ReadProcessSnapshot(ctx context.Context, runner Runner, key SortKey) (ProcessSnapshot, error) {
	output, err := runner.RunCommand(ctx, snapshotCommand(false), maxSnapshotOutput)
	legacy := false
	if unsupportedPSOptions(output) {
		legacy = true
		output, err = runner.RunCommand(ctx, snapshotCommand(true), maxSnapshotOutput)
	}
	if err != nil {
		return ProcessSnapshot{}, err
	}
	result := ProcessSnapshot{Truncated: strings.Count(strings.TrimSpace(string(output)), "\n")+1 >= snapshotReadRows}
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) >= maxSnapshotCommand+60 {
			result.CommandsTruncated = true
		}
	}
	if legacy {
		rows, parseErr := parseLegacyProcessRows(output, key, maxSnapshotProcesses+1, maxSnapshotCommand)
		if parseErr != nil {
			return ProcessSnapshot{}, parseErr
		}
		result.Processes = rows
	} else {
		firstBad := ""
		for _, line := range strings.Split(string(output), "\n") {
			line = strings.TrimSpace(strings.TrimRight(line, "\r"))
			if line == "" {
				continue
			}
			process, ok := parseProcessWithLimit(line, maxSnapshotCommand)
			if !ok {
				if firstBad == "" {
					firstBad = line
				}
				continue
			}
			if !ownPipeline(process.Command) {
				result.Processes = append(result.Processes, process)
			}
		}
		if len(result.Processes) == 0 {
			if firstBad != "" {
				return ProcessSnapshot{}, &Failure{Message: firstBad, Err: ErrUnsupported}
			}
			return ProcessSnapshot{}, ErrMalformed
		}
		sort.SliceStable(result.Processes, func(i, j int) bool {
			if key == SortMemory {
				return result.Processes[i].Memory > result.Processes[j].Memory
			}
			return result.Processes[i].CPU > result.Processes[j].CPU
		})
	}
	for _, process := range result.Processes {
		if len(process.Command) >= maxSnapshotCommand || strings.HasSuffix(process.Command, "…") {
			result.CommandsTruncated = true
		}
	}
	if len(result.Processes) > maxSnapshotProcesses {
		result.Truncated = true
		result.Processes = result.Processes[:maxSnapshotProcesses]
	}
	return result, nil
}
