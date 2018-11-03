package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/fatih/color"
)

type PendingRequests map[int64]*Event

var colors = []color.Attribute{
	color.FgWhite,
	color.FgBlue,
	color.FgHiBlue,
	color.FgGreen,
	color.FgHiGreen,
	color.FgYellow,
	color.FgHiYellow,
	color.FgCyan,
	color.FgHiCyan,
	color.FgMagenta,
	color.FgHiMagenta,
}

type Broker struct {
	Name             string
	InboundRequests  PendingRequests
	OutboundRequests PendingRequests
	Events           []*Event
	Color            *color.Color
	LastActivity     time.Time
}

func newBroker(name string) *Broker {
	return &Broker{
		Name:             name,
		InboundRequests:  make(PendingRequests),
		OutboundRequests: make(PendingRequests),
		Color:            color.New(colors[rand.Intn(len(colors))]),
		LastActivity:     time.Now().UTC(),
	}
}

func now() *time.Time {
	t := time.Now().UTC()
	return &t
}

func (b *Broker) GetRequest(inbound bool, id int64) *Event {
	if inbound {
		return b.InboundRequests[id]
	} else {
		return b.OutboundRequests[id]
	}
}

func (b *Broker) Retire() {
	for _, req := range b.InboundRequests {
		req.RecordCancellation()
	}
	for _, req := range b.OutboundRequests {
		req.RecordCancellation()
	}
}

func (b *Broker) Landed(ev *Event) {
	if ev.Inbound {
		delete(b.InboundRequests, ev.ID)
	} else {
		delete(b.OutboundRequests, ev.ID)
	}
}

func (b *Broker) Updated(ev *Event) {
	if !b.ShouldPrint(ev) {
		return
	}

	spacer := strings.Repeat("  ", len(b.InboundRequests)+len(b.OutboundRequests))
	arrow := "→"
	if ev.Inbound {
		arrow = "←"
	}
	b.Color.Printf("%s%s%s %s %s\n", b.Delta(), spacer, arrow, b.Name, ev)
}

func (b *Broker) Delta() string {
	s := ""
	d := time.Since(b.LastActivity)
	if d < 1*time.Millisecond {
		// nothing
	} else if d.Seconds() < 1.0 {
		// ms
		s = fmt.Sprintf("+%.f ms", d.Seconds()*1000)
	} else {
		// s
		s = fmt.Sprintf("+%.2f s", d.Seconds())
	}

	res := fmt.Sprintf("%10v ", s)
	b.LastActivity = time.Now().UTC()
	return res
}

var bannedMethods = []string{
	// "Meta.Authenticate",
	"Fetch.Commons",
	"Profile.Data",
}

func (b *Broker) ShouldPrint(ev *Event) bool {
	for _, banned := range bannedMethods {
		if strings.Contains(ev.Method, banned) {
			return false
		}
	}
	return true
}

type Event struct {
	Broker *Broker

	ID     int64      `json:"id"`
	Method string     `json:"method"`
	Start  *time.Time `json:"start"`
	End    *time.Time `json:"end"`
	Kind   EventKind  `json:"kind"`
	Raw    string     `json:"raw"`

	// When true, is a request/notif sent by the server to the client.
	// They're both peers, but conceptually teacup thinks of one as a server still.
	Inbound bool `json:"inbound"`

	Status EventStatus      `json:"status"`
	Error  *RpcError        `json:"error"`
	Params *json.RawMessage `json:"params"`
	Result *json.RawMessage `json:"result"`
}

func (ev *Event) AddTo(b *Broker) time.Time {
	ev.Broker = b
	b.Updated(ev)

	b.Events = append(b.Events, ev)
	if ev.Kind == EventKindRequest {
		if ev.Inbound {
			b.InboundRequests[ev.ID] = ev
		} else {
			b.OutboundRequests[ev.ID] = ev
		}
	}
	return time.Now().UTC()
}

func (ev *Event) RecordCompletion(result *json.RawMessage) {
	ev.End = now()
	ev.Result = result
	ev.Status = EventStatusCompleted

	b := ev.Broker
	b.Landed(ev)
	b.Updated(ev)
}

func (ev *Event) RecordError(err *RpcError) {
	ev.End = now()
	ev.Error = err
	ev.Status = EventStatusErrored

	b := ev.Broker
	b.Landed(ev)
	b.Updated(ev)
}

func (ev *Event) RecordCancellation() {
	ev.End = now()
	ev.Status = EventStatusCancelled

	b := ev.Broker
	b.Landed(ev)
	b.Updated(ev)
}

func (ev *Event) Duration() time.Duration {
	switch ev.Kind {
	case EventKindRequest:
		if ev.Start != nil && ev.End != nil {
			return ev.End.Sub(*ev.Start)
		}
		return time.Duration(0)
	case EventKindNotification:
		return time.Duration(0)
	}
	panic(fmt.Sprintf("Invalid event kind %s", ev.Kind))
}

func trim(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

func trimJSON(msg *json.RawMessage) string {
	if msg == nil {
		return "Ø"
	}
	bs := []byte(*msg)
	return trim(string(bs))
}

func (ev *Event) String() string {
	switch ev.Kind {
	case EventKindRequest:
		switch ev.Status {
		case EventStatusPending:
			return fmt.Sprintf("• [%d] %s %s", ev.ID, ev.Method, trimJSON(ev.Params))
		case EventStatusCompleted:
			return fmt.Sprintf("✔ [%d] %s (%s) %s", ev.ID, ev.Method, ev.Duration(), trimJSON(ev.Result))
		case EventStatusErrored:
			return fmt.Sprintf("✕ [%d] %s (%s) %s", ev.ID, ev.Method, ev.Duration(), trim(ev.Error.Message))
		case EventStatusCancelled:
			return fmt.Sprintf("⚐ [%d] %s (%s)", ev.ID, ev.Method, ev.Duration())
		}
	case EventKindNotification:
		if ev.Method == "Log" {
			var msg = struct {
				Level   string `json:"level"`
				Message string `json:"message"`
			}{}
			json.Unmarshal(*ev.Params, &msg)
			return fmt.Sprintf("# %s", msg.Message)
		}
		return fmt.Sprintf("- %s %s", ev.Method, trimJSON(ev.Params))
	}
	panic(fmt.Sprintf("Invalid event kind %s", ev.Kind))
}

type EventKind string

const (
	EventKindRequest      EventKind = "request"
	EventKindNotification EventKind = "notification"
)

type EventStatus string

const (
	EventStatusPending   EventStatus = "pending"
	EventStatusCompleted EventStatus = "completed"
	EventStatusErrored   EventStatus = "errored"
	EventStatusCancelled EventStatus = "cancelled"
)
