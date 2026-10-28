package workbench

import (
	"path"

	"motty/internal/term"
)

// Reports may arrive before the panel opens or while a listing/editor owns it.
// Consume only the latest one, once, rather than losing it or snapping back on
// every frame after the user has deliberately browsed somewhere else.
func (state *State) terminalDirectory(sessionID uint32, cwd term.Cwd) {
	session, ok := state.sessionByIDPointer(sessionID)
	if !ok || session.term == nil || !session.hasServer() || session.containerCommand != "" || state.sftpFollowOff {
		return
	}
	session.term.followPath = cwd.Path
}

func (state *State) followTerminalDirectory(session *Session) {
	t := session.term
	if t == nil || t.followPath == "" {
		return
	}
	if state.sftpFollowOff || !session.hasServer() || session.containerCommand != "" {
		t.followPath = ""
		return
	}
	view := session.files
	if view == nil || view.worker == nil || view.local || view.detached != nil || view.busy ||
		view.dialog != sftpDialogNone || view.editPath != "" || view.pathEditing || view.dropping {
		return
	}
	directory := t.followPath
	t.followPath = ""
	if path.Clean(view.path) != path.Clean(directory) {
		state.openSFTPDirectory(view, directory)
	}
}
