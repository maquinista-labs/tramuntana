package listener

import (
	"context"
	"log"
)

// Handler interfaces for event routing.
type ApprovalHandler interface {
	HandlePendingApproval(ev TaskEvent)
}

type QueueHandler interface {
	HandleTaskUpdate(ev TaskEvent)
}

type PlannerCrashHandler interface {
	HandlePlannerCrash(ev PlannerEvent)
}

// EventRouter dispatches events from the listener to registered handlers.
type EventRouter struct {
	listener *EventListener
	approval ApprovalHandler
	queue    QueueHandler
	crash    PlannerCrashHandler
}

// NewRouter creates an EventRouter wired to the given listener and handlers.
// Any handler may be nil (events for that handler will be logged and skipped).
func NewRouter(l *EventListener, approval ApprovalHandler, queue QueueHandler, crash PlannerCrashHandler) *EventRouter {
	return &EventRouter{
		listener: l,
		approval: approval,
		queue:    queue,
		crash:    crash,
	}
}

// Run starts dispatching events. Blocks until context is cancelled.
func (r *EventRouter) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case ev := <-r.listener.TaskEvents:
			switch ev.Status {
			case "pending_approval":
				if r.approval != nil {
					r.approval.HandlePendingApproval(ev)
				} else {
					log.Printf("router: no approval handler for task %s", ev.TaskID)
				}
			default:
				// done, ready, failed, claimed, etc.
				if r.queue != nil {
					r.queue.HandleTaskUpdate(ev)
				}
			}

		case ev := <-r.listener.PlannerEvents:
			if ev.Status == "crashed" {
				if r.crash != nil {
					r.crash.HandlePlannerCrash(ev)
				} else {
					log.Printf("router: no crash handler for planner session %s", ev.SessionID)
				}
			}
		}
	}
}
