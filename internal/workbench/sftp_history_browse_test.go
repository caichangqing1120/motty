package workbench

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSFTPHistoryPairedBrowseKeepsBothTimelinesAndBack(t *testing.T) {
	state, window, _, host, root := dualSFTPFixture(t, 1)
	preview := host.windows[0].view
	child := filepath.Join(root, "docs")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	window.toggleBrowseSync()
	window.navigate(false, "/home/test/docs")
	dualWait(t, state, window, preview, func() bool {
		return !window.navigationBusy() && window.view.lfolder.path == child
	})
	for side, folder := range []*sftpFolder{&window.view.lfolder, &window.view.sftpFolder} {
		if len(folder.history.entries) < 2 || folder.history.entries[folder.history.cursor].path != folder.path {
			t.Fatalf("paired browse did not record side %d: path=%q history=%+v", side, folder.path, folder.history)
		}
	}
	if !state.navigateSFTPHistory(window.view, false, -1) {
		t.Fatal("paired Back unavailable")
	}
	dualWait(t, state, window, preview, func() bool {
		return !window.navigationBusy() && window.view.path == "/home/test" && window.view.lfolder.path == root
	})
	if !window.browse.enabled || len(window.view.worker.Transfers(nil)) != 0 {
		t.Fatal("history bypassed paired navigation or changed transfers")
	}
}
