package workbench

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"motty/internal/sessionlog"
	"motty/internal/sshconn"
)

// Session logs are the user's view of "what did I connect to, and how did it
// end": one bounded, local row per logical session, fed from the same lifecycle
// transitions the terminal already follows. The database is a separate private
// SQLite file, never part of the configuration, the command history, a backup or
// a sync; see internal/sessionlog for what may and may not be stored.
//
// Two rules keep the rest of the workbench honest:
//   - no database call happens on the paint path, and
//   - a State built by hand (most tests) simply has no store, and every helper
//     here stays nil-safe.

// sessionLogs is the metadata journal owned by one window.
type sessionLogs struct {
	store *sessionlog.Store
	queue *sessionlog.Queue
	// runID names this process run. It is the same for every window of the
	// process, so one window's startup recovery never marks another window's
	// live row interrupted.
	runID   string
	enabled bool
	openErr error
	view    sessionLogsView
	// results carries query completions from the worker goroutines back to the
	// window thread; done is closed when the window stops caring.
	results chan sessionLogsResult
	done    chan struct{}
	// requestGeneration drops stale query results; the view keeps the version it
	// asked for and ignores everything older.
	requestGeneration uint64
}

// sessionLogsView is what the page paints: the current query, the rows it
// produced, and the detail selection. Paint only reads it; workers rebuild it.
type sessionLogsView struct {
	query   sessionlog.Query
	rows    []sessionlog.Record
	next    sessionlog.Cursor
	more    bool
	loading bool
	err     string

	unavailable bool

	selected     string
	events       []sessionlog.Event
	eventsFor    string
	eventsMore   bool
	eventsErr    string
	eventsLoad   bool
	eventGen     uint64
	selectedGen  uint64
	selectedLive bool
}

// sessionLogsResult is one background query completion. Exactly one of page and
// events is set.
type sessionLogsResult struct {
	generation uint64
	page       sessionlog.Page
	events     []sessionlog.Event
	eventsFor  string
	eventsGen  uint64
	err        error
}

// sessionLogProcessRunID is the process-wide run identity used when Options does
// not inject one. A failure to draw random bytes still yields a distinct value.
var sessionLogProcessRunID = sync.OnceValue(func() string {
	if id, err := sessionlog.NewID(); err == nil {
		return id
	}
	return fmt.Sprintf("run-%d", time.Now().UnixNano())
})

// initSessionLogs opens this window's journal, recovers rows a previous process
// run left open, and starts the ordered writer. A database that cannot be opened
// is remembered and reported, never fatal for connections.
func (state *State) initSessionLogs() {
	state.sessionLogs.enabled = !state.sessionLogsOff
	state.sessionLogs.runID = state.options.SessionLogRunID
	if state.sessionLogs.runID == "" {
		state.sessionLogs.runID = sessionLogProcessRunID()
	}
	state.sessionLogs.results = make(chan sessionLogsResult, 8)
	state.sessionLogs.done = make(chan struct{})
	store, err := sessionlog.Open(state.sessionLogPath())
	if err != nil {
		state.sessionLogs.openErr = err
		state.sessionLogs.view.unavailable = true
		state.reportSessionLogProblem("open", err)
		return
	}
	state.sessionLogs.store = store
	state.sessionLogs.queue = sessionlog.NewQueue(store)
	if err := store.Recover(state.sessionLogs.runID, state.wallNow()); err != nil {
		state.reportSessionLogProblem("recover", err)
	}
}

// sessionLogPath is the private journal, beside the saved state and apart from
// every other database.
func (state *State) sessionLogPath() string {
	return filepath.Join(filepath.Dir(state.options.StatePath), "session-logs.db")
}

// sessionLogsRecording reports whether new lifecycle rows may be written.
func (state *State) sessionLogsRecording() bool {
	return state.sessionLogs.enabled && state.sessionLogs.store != nil && state.sessionLogs.queue != nil
}

// wallNow is the day-and-time clock, nil-safe for a State built by hand.
func (state *State) wallNow() time.Time {
	if state.options.WallNow == nil {
		return time.Now()
	}
	return state.options.WallNow()
}

// reportSessionLogProblem writes a store failure to the diagnostic log. The
// error text is a database or file-system message, never terminal content or an
// authentication answer; the page shows the user that records are unavailable.
func (state *State) reportSessionLogProblem(stage string, err error) {
	if err == nil || state.options.Log == nil {
		return
	}
	fmt.Fprintf(state.options.Log, "session log %s: %v\n", stage, err)
}

// submitSessionLog hands one write to the ordered writer. The acknowledgement is
// drained on a goroutine; the connection path never waits for SQLite.
func (state *State) submitSessionLog(operation sessionlog.Operation) {
	queue := state.sessionLogs.queue
	if queue == nil {
		return
	}
	ack := queue.Submit(operation)
	go func() {
		if err := <-ack; err != nil {
			state.reportSessionLogProblem("write", err)
		}
	}()
}

// startSessionLog opens a row for a user-dialed session. A manual redial is a
// new logical session, so any row the tab still owns is finalized first.
func (state *State) startSessionLog(session *Session, host Host) {
	if session == nil {
		return
	}
	state.finishSessionLog(session, sessionlog.StatusEnded, sessionlog.ReasonUserClosed)
	if !state.sessionLogsRecording() {
		return
	}
	id, err := sessionlog.NewID()
	if err != nil {
		state.reportSessionLogProblem("id", err)
		return
	}
	record := sessionlog.Record{
		ID:        id,
		RunID:     state.sessionLogs.runID,
		Host:      sessionLogHostName(host),
		Address:   sessionLogText(host.Address),
		Account:   sessionLogText(state.login(host)),
		Protocol:  sessionLogProtocol(host),
		StartedAt: state.wallNow().UTC(),
	}
	session.logRecordID = id
	session.logFinished = false
	session.logStatus = sessionlog.StatusConnecting
	state.submitSessionLog(sessionlog.Operation{Kind: sessionlog.OpStart, Record: record})
}

// startSharedSessionLog records a tab that borrows an already connected
// transport (a container shell): the transport is up when the tab appears.
func (state *State) startSharedSessionLog(session *Session, host Host) {
	state.startSessionLog(session, host)
	if session.logRecordID == "" {
		return
	}
	state.submitSessionLogEvent(session, sessionlog.EventConnected, 0)
	session.logStatus = sessionlog.StatusActive
}

// submitSessionLogEvent appends one typed event to the session's timeline.
func (state *State) submitSessionLogEvent(session *Session, kind sessionlog.EventKind, attempt int) {
	if session == nil || session.logRecordID == "" || session.logFinished || !state.sessionLogsRecording() {
		return
	}
	state.submitSessionLog(sessionlog.Operation{
		Kind: sessionlog.OpAppendEvent,
		Event: sessionlog.Event{
			RecordID: session.logRecordID,
			Kind:     kind,
			At:       state.wallNow().UTC(),
			Attempt:  attempt,
		},
	})
}

// finishSessionLog writes the terminal status once. Repeated and late calls do
// nothing, so every close path may call it without coordinating.
func (state *State) finishSessionLog(session *Session, status sessionlog.Status, reason sessionlog.Reason) {
	if session == nil || session.logRecordID == "" || session.logFinished {
		return
	}
	session.logFinished = true
	session.logStatus = status
	if !state.sessionLogsRecording() {
		return
	}
	state.submitSessionLog(sessionlog.Operation{
		Kind:    sessionlog.OpFinish,
		ID:      session.logRecordID,
		Status:  status,
		EndedAt: state.wallNow().UTC(),
		Reason:  reason,
	})
}

// journalConnectionEvent mirrors the in-memory connection story into the typed
// timeline. The story keeps its existing contract; this only adds a known event
// kind, never the free-text detail.
func (state *State) journalConnectionEvent(session *Session, kind connectionEventKind) {
	if session == nil || session.logRecordID == "" || session.logFinished {
		return
	}
	switch kind {
	case connectionEventConnected:
		// journalConnectionOutcome records it once the endpoint role is known.
	case connectionEventClosed:
		state.submitSessionLogEvent(session, sessionlog.EventDisconnected, 0)
	case connectionEventFailed:
		state.submitSessionLogEvent(session, sessionlog.EventFailed, 0)
	case connectionEventReconnectWaiting:
		session.logStatus = sessionlog.StatusReconnecting
		state.submitSessionLogEvent(session, sessionlog.EventReconnectScheduled, session.reconnectAttempt)
	case connectionEventReconnectStarted:
		session.logStatus = sessionlog.StatusReconnecting
		state.submitSessionLogEvent(session, sessionlog.EventReconnectStarted, session.reconnectAttempt)
	case connectionEventReconnectStopped:
		state.submitSessionLogEvent(session, sessionlog.EventReconnectExhausted, session.reconnectAttempt)
	case connectionEventReconnectCanceled:
		state.submitSessionLogEvent(session, sessionlog.EventReconnectCanceled, session.reconnectAttempt)
		state.finishSessionLog(session, sessionlog.StatusFailed, sessionlog.ReasonCanceled)
	}
}

// journalConnectionOutcome decides, after the reconnect policy has run, whether
// a status is final. A transient close with a retry pending keeps the row alive
// as reconnecting; everything else ends it truthfully.
func (state *State) journalConnectionOutcome(session *Session, status sshconn.Status) {
	if session == nil || session.logRecordID == "" {
		return
	}
	switch status.Phase {
	case sshconn.PhaseConnected:
		if status.EndpointRole == sshconn.EndpointProxyJump || session.logFinished {
			return
		}
		if session.logStatus == sessionlog.StatusActive {
			return
		}
		state.submitSessionLogEvent(session, sessionlog.EventConnected, 0)
		session.logStatus = sessionlog.StatusActive
	case sshconn.PhaseClosed, sshconn.PhaseFailed:
		if session.logFinished {
			return
		}
		if session.reconnectPending {
			session.logStatus = sessionlog.StatusReconnecting
			return
		}
		outcome, reason := sessionLogOutcome(status)
		state.finishSessionLog(session, outcome, reason)
	}
}

// sessionLogOutcome maps a terminal connection status to a stored status and a
// known reason code. It never returns the connection's error text.
func sessionLogOutcome(status sshconn.Status) (sessionlog.Status, sessionlog.Reason) {
	switch {
	case status.Err == nil:
		return sessionlog.StatusEnded, sessionlog.ReasonNone
	case errors.Is(status.Err, sshconn.ErrAuthFailed):
		return sessionlog.StatusFailed, sessionlog.ReasonAuthFailed
	case errors.Is(status.Err, sshconn.ErrHostKeyRejected):
		return sessionlog.StatusFailed, sessionlog.ReasonHostKeyRejected
	case errors.Is(status.Err, sshconn.ErrCanceled):
		return sessionlog.StatusEnded, sessionlog.ReasonCanceled
	case errors.Is(status.Err, sshconn.ErrKeepAliveTimeout):
		return sessionlog.StatusFailed, sessionlog.ReasonTimeout
	case errors.Is(status.Err, sshconn.ErrDisconnected):
		return sessionlog.StatusFailed, sessionlog.ReasonNetworkLost
	case errors.Is(status.Err, sshconn.ErrHostKeyChanged):
		return sessionlog.StatusFailed, sessionlog.ReasonConnectFailed
	default:
		return sessionlog.StatusFailed, sessionlog.ReasonConnectFailed
	}
}

// setSessionLogsEnabled turns metadata capture on or off. Disabling never
// touches a live connection: rows of sessions that are still open are closed as
// "tracking stopped", which says the capture ended, not the session. Enabling
// again only records sessions opened afterwards.
func (state *State) setSessionLogsEnabled(on bool) {
	if state.sessionLogs.enabled == on {
		return
	}
	if !on {
		// Close the open rows while the writer still accepts them, then stop
		// taking new ones.
		for index := range state.sessions {
			session := &state.sessions[index]
			if session.logRecordID == "" || session.logFinished {
				continue
			}
			state.submitSessionLogEvent(session, sessionlog.EventTrackingStopped, 0)
			state.finishSessionLog(session, sessionlog.StatusTrackingStopped, sessionlog.ReasonTrackingOff)
		}
	}
	state.sessionLogs.enabled = on
}

// closeSessionLogs flushes pending rows and closes the journal. Call it after
// the window's sessions are finalized: the queue drains before its store closes.
func (state *State) closeSessionLogs() {
	if state.sessionLogs.done != nil {
		select {
		case <-state.sessionLogs.done:
		default:
			close(state.sessionLogs.done)
		}
	}
	if state.sessionLogs.queue != nil {
		if err := state.sessionLogs.queue.Close(); err != nil {
			state.reportSessionLogProblem("flush", err)
		}
		state.sessionLogs.queue = nil
	}
	if state.sessionLogs.store != nil {
		if err := state.sessionLogs.store.Close(); err != nil {
			state.reportSessionLogProblem("close", err)
		}
		state.sessionLogs.store = nil
	}
}

// refreshSessionLogs asks a worker for the page the view currently describes.
// Late answers are dropped by generation, so typing or filtering never lets an
// older result replace a newer one.
func (state *State) refreshSessionLogs() {
	view := &state.sessionLogs.view
	state.sessionLogs.requestGeneration++
	view.loading = true
	view.err = ""
	store := state.sessionLogs.store
	if store == nil {
		view.unavailable = true
		view.rows = nil
		view.more = false
		view.loading = false
		return
	}
	generation := state.sessionLogs.requestGeneration
	query := view.query
	go func() {
		page, err := store.List(query)
		state.postSessionLogsResult(sessionLogsResult{generation: generation, page: page, err: err})
	}()
}

// loadSessionLogEvents asks a worker for the selected record's timeline.
func (state *State) loadSessionLogEvents(id string) {
	view := &state.sessionLogs.view
	store := state.sessionLogs.store
	view.selected = id
	view.events = nil
	view.eventsMore = false
	view.eventsErr = ""
	view.eventsLoad = store != nil
	view.eventGen++
	if store == nil || id == "" {
		view.eventsLoad = false
		return
	}
	generation := view.eventGen
	go func() {
		events, err := store.Events(id)
		state.postSessionLogsResult(sessionLogsResult{eventsFor: id, eventsGen: generation, events: events, err: err})
	}()
}

// postSessionLogsResult hands a worker's answer to the window thread.
func (state *State) postSessionLogsResult(result sessionLogsResult) {
	if state.options.Invalidate != nil {
		state.options.Invalidate()
	}
	select {
	case <-state.sessionLogs.done:
	case state.sessionLogs.results <- result:
	}
}

// pumpSessionLogs runs completed queries. Wake calls it; Paint never does.
func (state *State) pumpSessionLogs() {
	if state.sessionLogs.results == nil {
		return
	}
	for {
		select {
		case result := <-state.sessionLogs.results:
			state.applySessionLogsResult(result)
		default:
			return
		}
	}
}

func (state *State) applySessionLogsResult(result sessionLogsResult) {
	view := &state.sessionLogs.view
	if result.eventsFor != "" || result.eventsGen != 0 {
		if result.eventsGen != view.eventGen || result.eventsFor != view.selected {
			return
		}
		view.eventsLoad = false
		view.events = result.events
		if record, ok := state.sessionLogRecord(view.selected); ok {
			view.eventsMore = record.Truncated
		}
		if result.err != nil {
			view.eventsErr = "unavailable"
			state.reportSessionLogProblem("events", result.err)
		}
		return
	}
	if result.generation != state.sessionLogs.requestGeneration {
		return
	}
	view.loading = false
	if result.err != nil {
		view.err = "unavailable"
		view.rows = nil
		view.more = false
		state.reportSessionLogProblem("list", result.err)
		return
	}
	view.rows = result.page.Records
	view.next = result.page.Next
	view.more = result.page.More
	view.unavailable = false
}

// setSessionLogFavorite marks one record, without touching any other history.
func (state *State) setSessionLogFavorite(id string, on bool) {
	if state.sessionLogs.queue == nil {
		return
	}
	state.submitSessionLog(sessionlog.Operation{Kind: sessionlog.OpSetFavorite, ID: id, Favorite: on})
}

// deleteSessionLog removes one record and its timeline.
func (state *State) deleteSessionLog(id string) {
	if state.sessionLogs.queue == nil {
		return
	}
	state.submitSessionLog(sessionlog.Operation{Kind: sessionlog.OpDelete, ID: id})
}

// clearSessionLogs removes this journal only: command history, manual terminal
// recordings and the diagnostic log are separate files and stay untouched.
func (state *State) clearSessionLogs(includeFavorites bool) {
	if state.sessionLogs.queue == nil {
		return
	}
	state.submitSessionLog(sessionlog.Operation{Kind: sessionlog.OpClear, IncludeFavorites: includeFavorites})
}

// sessionLogRecord finds a row already loaded into the view.
func (state *State) sessionLogRecord(id string) (sessionlog.Record, bool) {
	for _, record := range state.sessionLogs.view.rows {
		if record.ID == id {
			return record, true
		}
	}
	return sessionlog.Record{}, false
}

// sessionForLogRecord resolves a still-open tab by its stable record ID. A
// closed or moved tab whose integer session ID was reused is never matched.
func (state *State) sessionForLogRecord(id string) *Session {
	if id == "" {
		return nil
	}
	for index := range state.sessions {
		if state.sessions[index].logRecordID == id && !state.sessions[index].logFinished {
			return &state.sessions[index]
		}
	}
	return nil
}

// sessionLogHostName is the bounded display name of a host: its label, or the
// address when it has none.
func sessionLogHostName(host Host) string {
	switch {
	case strings.TrimSpace(host.Label) != "":
		return sessionLogText(host.Label)
	case strings.TrimSpace(host.Address) != "":
		return sessionLogText(host.Address)
	default:
		return "session"
	}
}

// sessionLogText makes a display snapshot safe to store: valid UTF-8 without
// control characters, which is also how terminal bytes and multi-line server
// text would look.
func sessionLogText(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
}

// sessionLogProtocol maps the workbench's host protocol to the stored enum.
func sessionLogProtocol(host Host) sessionlog.Protocol {
	if host.ID == localHostID {
		return sessionlog.ProtocolLocal
	}
	switch host.Protocol {
	case protocolTelnet:
		return sessionlog.ProtocolTelnet
	case protocolSerial:
		return sessionlog.ProtocolSerial
	case protocolVNC:
		return sessionlog.ProtocolVNC
	case protocolRDP:
		return sessionlog.ProtocolRDP
	default:
		return sessionlog.ProtocolSSH
	}
}

// broadcastSessionLogsEnabled tells every window of the process about a capture
// switch. All windows run on the event-loop thread, so the peers are mutated
// directly, as the other cross-window settings do.
func (group *Group) broadcastSessionLogsEnabled(enabled bool) {
	group.mu.Lock()
	members := append([]*State(nil), group.members...)
	group.mu.Unlock()
	for _, member := range members {
		member.setSessionLogsEnabled(enabled)
		member.invalidateWindow()
	}
}

// invalidateWindow asks the runtime for one repaint when there is a window.
func (state *State) invalidateWindow() {
	if state.options.Invalidate != nil {
		state.options.Invalidate()
	}
}
