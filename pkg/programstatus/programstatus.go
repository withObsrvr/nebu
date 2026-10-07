// Package programstatus emits Program Status Protocol (OSC 7501) reports.
package programstatus

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

// State is a Program Status Protocol state.
type State string

const (
	Idle    State = "idle"
	Working State = "working"
	Done    State = "done"
	Blocked State = "blocked"
	Error   State = "error"
	Clear   State = "clear"
)

var (
	appPattern = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,32}$`)
	idPattern  = regexp.MustCompile(`^[A-Za-z0-9_.+-]{1,32}(?:/[A-Za-z0-9_.+-]{1,32}){0,7}$`)
)

// Report is one OSC 7501 status report. Progress is omitted when nil.
type Report struct {
	State    State
	ID       string
	Kind     string
	Progress *int
	App      string
	Title    string
	Message  string
}

// Write validates and writes one OSC 7501 report to w.
func Write(w io.Writer, report Report) error {
	body, err := encode(report)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "\x1b]7501;"+body+"\x1b\\")
	return err
}

func encode(report Report) (string, error) {
	switch report.State {
	case Idle, Working, Done, Blocked, Error, Clear:
	default:
		return "", fmt.Errorf("invalid program status state %q", report.State)
	}

	pairs := []string{"state=" + string(report.State)}
	if report.ID != "" {
		if len(report.ID) > 128 || !idPattern.MatchString(report.ID) {
			return "", fmt.Errorf("invalid program status id %q", report.ID)
		}
		pairs = append(pairs, "id="+report.ID)
	}
	if report.Kind != "" {
		if report.State != Blocked {
			return "", errors.New("program status kind requires blocked state")
		}
		switch report.Kind {
		case "permission", "question", "auth":
		default:
			return "", fmt.Errorf("invalid program status kind %q", report.Kind)
		}
		pairs = append(pairs, "kind="+report.Kind)
	}
	if report.Progress != nil {
		if report.State != Working && report.State != Blocked {
			return "", errors.New("program status progress requires working or blocked state")
		}
		if *report.Progress < 0 || *report.Progress > 100 {
			return "", fmt.Errorf("program status progress %d is outside 0..100", *report.Progress)
		}
		pairs = append(pairs, fmt.Sprintf("progress=%d", *report.Progress))
	}
	if report.App != "" {
		if !appPattern.MatchString(report.App) {
			return "", fmt.Errorf("invalid program status app %q", report.App)
		}
		pairs = append(pairs, "app="+report.App)
	}
	if report.Title != "" {
		encoded, err := encodeText("title", report.Title, 192)
		if err != nil {
			return "", err
		}
		pairs = append(pairs, "title="+encoded)
	}
	if report.Message != "" {
		encoded, err := encodeText("message", report.Message, 2048)
		if err != nil {
			return "", err
		}
		pairs = append(pairs, "msg="+encoded)
	}

	sequence := "\x1b]7501;" + strings.Join(pairs, ":") + "\x1b\\"
	if len(sequence) > 4096 {
		return "", errors.New("program status report exceeds 4096 bytes")
	}
	return strings.Join(pairs, ":"), nil
}

func encodeText(name, value string, limit int) (string, error) {
	if len(value) > limit {
		return "", fmt.Errorf("program status %s exceeds %d bytes", name, limit)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("program status %s contains a control character", name)
		}
	}
	return base64.StdEncoding.EncodeToString([]byte(value)), nil
}
