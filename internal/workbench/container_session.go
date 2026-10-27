package workbench

import "errors"

// openContainerTab borrows the selected tab's authenticated SSH transport, never
// another matching host or a new login. The command is constructed by remotetools.
func (state *State) openContainerTab(sourceID uint32, _ string, command string) error {
	source, ok := state.sessionByIDPointer(sourceID)
	if !ok || !state.remoteToolsOffered(source) || !source.host.isSSH() {
		return errors.New("the SSH host is no longer connected")
	}
	lender, ok := source.conn.(sharer)
	if !ok {
		return errors.New("this SSH connection cannot open another terminal")
	}
	relay := newNotifyRelay(state.options.Invalidate)
	conn, err := lender.Share(relay.call)
	if err != nil {
		return err
	}
	if state.createHostSessionWithConnection(source.host, source.Address, conn, command, relay) == nil {
		conn.Cancel()
		return errors.New("unable to create a new terminal tab")
	}
	return nil
}
