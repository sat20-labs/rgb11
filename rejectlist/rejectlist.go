// Package rejectlist implements the advisory IFA RejectListUrl policy used by
// interoperable RGB wallets. It is wallet policy, not RGB consensus.
package rejectlist

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sat20-labs/rgb11/operations"
)

const MaxResponseBytes int64 = 1 << 20

var (
	ErrService       = errors.New("RGB11 reject list service unavailable")
	ErrURLPolicy     = errors.New("RGB11 reject list URL violates transport policy")
	ErrResponseLimit = errors.New("RGB11 reject list response exceeds limit")
)

// List is the normalized official line-oriented policy. A later occurrence of
// the same Opout wins, matching the reference implementation.
type List struct {
	Reject map[operations.Opout]struct{}
	Allow  map[operations.Opout]struct{}
}

// Parse ignores blank, comment and malformed lines. Prefixing a valid Opout
// with '!' explicitly allows it and shields its ancestors for that DAG path.
func Parse(raw []byte) List {
	decisions := make(map[operations.Opout]bool)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), int(MaxResponseBytes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		allowed := strings.HasPrefix(line, "!")
		if allowed {
			line = strings.TrimSpace(strings.TrimPrefix(line, "!"))
		}
		opout, err := operations.ParseOpout(line)
		if err != nil {
			continue
		}
		decisions[opout] = allowed
	}
	list := List{Reject: make(map[operations.Opout]struct{}), Allow: make(map[operations.Opout]struct{})}
	for opout, allowed := range decisions {
		if allowed {
			list.Allow[opout] = struct{}{}
		} else {
			list.Reject[opout] = struct{}{}
		}
	}
	return list
}

// RejectedAncestor returns the first unshielded rejected Opout reachable from
// checked. An explicitly allowed Opout stops traversal of that ancestry path.
func (l List) RejectedAncestor(checked operations.Opout,
	dag map[operations.Opout][]operations.Opout) (operations.Opout, bool) {
	stack := []operations.Opout{checked}
	seen := make(map[operations.Opout]struct{})
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		if _, allowed := l.Allow[current]; allowed {
			continue
		}
		if _, rejected := l.Reject[current]; rejected {
			return current, true
		}
		stack = append(stack, dag[current]...)
	}
	return operations.Opout{}, false
}

// Client fetches one bounded list under the wallet's transport policy.
type Client struct {
	HTTP              *http.Client
	AllowLoopbackHTTP bool
}

func (c Client) Fetch(ctxURL string) (List, error) {
	u, err := validateURL(ctxURL, c.AllowLoopbackHTTP)
	if err != nil {
		return List{}, err
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	originalScheme, originalHost := u.Scheme, u.Host
	copyClient := *client
	if copyClient.Timeout <= 0 {
		copyClient.Timeout = 10 * time.Second
	}
	copyClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Scheme != originalScheme || !strings.EqualFold(req.URL.Host, originalHost) {
			return ErrURLPolicy
		}
		_, err := validateURL(req.URL.String(), c.AllowLoopbackHTTP)
		return err
	}
	request, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return List{}, fmt.Errorf("%w: %v", ErrService, err)
	}
	response, err := copyClient.Do(request)
	if err != nil {
		return List{}, fmt.Errorf("%w: %v", ErrService, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return List{}, fmt.Errorf("%w: HTTP %d", ErrService, response.StatusCode)
	}
	limited := io.LimitReader(response.Body, MaxResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return List{}, fmt.Errorf("%w: %v", ErrService, err)
	}
	if int64(len(raw)) > MaxResponseBytes {
		return List{}, ErrResponseLimit
	}
	return Parse(raw), nil
}

func validateURL(raw string, allowLoopbackHTTP bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, ErrURLPolicy
	}
	switch u.Scheme {
	case "https":
		return u, nil
	case "http":
		host := strings.ToLower(u.Hostname())
		if allowLoopbackHTTP && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
			return u, nil
		}
	}
	return nil, ErrURLPolicy
}
