package reconcile

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

var _ Reconciler = (*Ufw)(nil)

type Ufw struct {
	AllowedTcpPorts []int
}

func (r *Ufw) Reconcile(ctx context.Context) error {
	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("ufw not found: %w", err)
	}

	// Normalize desired ports (unique, sorted)
	desiredPorts := uniqueInts(r.AllowedTcpPorts)
	sort.Ints(desiredPorts)

	// Check whether ufw is already active so we can preload rules before
	// enabling it for the first time. This avoids the situation where we
	// enable ufw (which defaults to deny incoming) before our allow rules
	// are in place, and then fail, locking the server out.
	active, err := r.isActive(ctx)
	if err != nil {
		return fmt.Errorf("check ufw status: %w", err)
	}

	if !active {
		for _, p := range desiredPorts {
			spec := fmt.Sprintf("%d/tcp", p)
			if err := r.execRun(ctx, "ufw", "allow", spec); err != nil {
				return fmt.Errorf("pre-allow %s before enabling ufw: %w", spec, err)
			}
		}
		if err := r.execRun(ctx, "ufw", "--force", "enable"); err != nil {
			return fmt.Errorf("enable ufw: %w", err)
		}
	}

	// Get current rules, numbered, to allow deletes by number.
	out, err := r.execCapture(ctx, "ufw", "status", "numbered")
	if err != nil {
		return fmt.Errorf("ufw status: %w", err)
	}
	current := parseUfwStatusNumbered(out)

	// Compute desired set: for each port, want both v4 and v6 allows (tcp).
	type key struct {
		Port int
		V6   bool
	}
	desired := make(map[key]struct{}, len(desiredPorts)*2)
	for _, p := range desiredPorts {
		desired[key{Port: p, V6: false}] = struct{}{}
		desired[key{Port: p, V6: true}] = struct{}{}
	}

	// Determine adds and deletes.
	// Track present TCP allows (both v4/v6) and their rule numbers.
	currentAllows := make(map[key][]int)
	var toDelete []int
	for _, rule := range current {
		if !strings.EqualFold(rule.Action, "allow") {
			continue
		}
		if rule.Direction != "" && !strings.EqualFold(rule.Direction, "in") {
			continue
		}

		if rule.Protocol == "tcp" && !rule.Range && rule.Service == "" && rule.Port > 0 {
			k := key{Port: rule.Port, V6: rule.V6}
			if _, ok := desired[k]; ok {
				currentAllows[k] = append(currentAllows[k], rule.Number)
				continue
			}
		}

		toDelete = append(toDelete, rule.Number)
	}

	// Adds: any desired not present
	var toAdd []key
	for k := range desired {
		if _, ok := currentAllows[k]; !ok {
			toAdd = append(toAdd, k)
		}
	}

	// Deletes: drop duplicate rules for ports we manage.
	for _, nums := range currentAllows {
		if len(nums) > 1 {
			toDelete = append(toDelete, nums[1:]...)
		}
	}
	// Delete from highest to lowest to keep numbering stable
	sort.Sort(sort.Reverse(sort.IntSlice(toDelete)))

	// Apply deletions
	for _, n := range toDelete {
		if err := r.execRun(ctx, "ufw", "--force", "delete", strconv.Itoa(n)); err != nil {
			return fmt.Errorf("ufw delete rule %d: %w", n, err)
		}
	}

	// Apply additions
	for _, k := range toAdd {
		spec := fmt.Sprintf("%d/tcp", k.Port)
		if err := r.execRun(ctx, "ufw", "allow", spec); err != nil {
			return fmt.Errorf("ufw allow %s: %w", spec, err)
		}
	}

	if err := r.execRun(ctx, "ufw", "status", "verbose"); err != nil {
		return fmt.Errorf("ufw status verbose: %w", err)
	}
	return nil
}

func (r *Ufw) execCapture(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func (r *Ufw) execRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *Ufw) isActive(ctx context.Context) (bool, error) {
	out, err := r.execCapture(ctx, "ufw", "status")
	if err != nil {
		return false, err
	}

	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "status:") {
			continue
		}
		lower := strings.ToLower(line)
		switch {
		case strings.Contains(lower, "inactive"):
			return false, nil
		case strings.Contains(lower, "active"):
			return true, nil
		default:
			return false, fmt.Errorf("unexpected ufw status line: %s", line)
		}
	}

	return false, fmt.Errorf("could not determine ufw status from output: %q", strings.TrimSpace(string(out)))
}

type ufwRule struct {
	Number    int
	To        string
	Action    string
	Direction string
	From      string
	V6        bool
	Port      int
	Protocol  string // "tcp", "udp", or empty
	Range     bool
	Service   string // non-numeric "To" (like "OpenSSH")
}

func parseUfwStatusNumbered(b []byte) []ufwRule {
	var rules []ufwRule
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Lines look like:
		// [ 1] 22/tcp                     ALLOW IN    Anywhere
		// [ 2] 22/tcp (v6)                ALLOW IN    Anywhere (v6)
		// [ 3] OpenSSH                    ALLOW IN    Anywhere
		// Header lines contain "Status:" or "To" "Action" "From" etc.; skip non-rule lines.
		if !strings.HasPrefix(line, "[") {
			continue
		}
		// Extract number in brackets
		r := ufwRule{}
		right := line
		if i := strings.Index(line, "]"); i != -1 && strings.HasPrefix(line, "[") {
			numStr := strings.TrimSpace(line[1:i])
			n, _ := strconv.Atoi(strings.TrimSpace(numStr))
			r.Number = n
			right = strings.TrimSpace(line[i+1:])
		} else {
			continue
		}
		// Split remaining columns into tokens to capture action/direction/from.
		fields := strings.Fields(right)
		if len(fields) < 2 {
			continue
		}
		r.To = fields[0]
		r.Action = fields[1]
		if len(fields) >= 3 {
			dir := strings.ToUpper(fields[2])
			if dir == "IN" || dir == "OUT" {
				r.Direction = dir
				if len(fields) >= 4 {
					r.From = strings.Join(fields[3:], " ")
				}
			} else {
				r.From = strings.Join(fields[2:], " ")
			}
		}
		if r.From == "" && len(fields) > 2 {
			r.From = strings.Join(fields[2:], " ")
		}
		r.From = strings.TrimSpace(r.From)

		// Determine v6 marker
		if strings.Contains(r.To, "(v6)") || strings.Contains(r.From, "(v6)") {
			r.V6 = true
		}

		// Parse To into port/protocol when numeric
		to := strings.TrimSpace(strings.ReplaceAll(r.To, "(v6)", ""))
		// Either "22/tcp" or "80" or "OpenSSH" or "1000:2000/tcp"
		if n, proto, isRange, ok := parsePortProto(to); ok {
			r.Port = n
			r.Protocol = proto
			r.Range = isRange
		} else {
			// Non-numeric service name
			r.Service = strings.TrimSpace(to)
		}

		rules = append(rules, r)
	}

	return rules
}

func parsePortProto(s string) (port int, proto string, isRange bool, ok bool) {
	// Examples: "22/tcp", "22", "1000:2000/tcp"
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	parts := strings.Split(s, "/")
	base := parts[0]
	if len(parts) > 1 {
		proto = strings.ToLower(parts[1])
	}
	if strings.Contains(base, ":") {
		// range: "1000:2000"
		isRange = true
		return 0, proto, true, false
	}
	n, err := strconv.Atoi(base)
	if err != nil {
		return 0, proto, false, false
	}
	return n, proto, false, true
}

func uniqueInts(in []int) []int {
	seen := make(map[int]struct{}, len(in))
	var out []int
	for _, v := range in {
		if v <= 0 {
			continue
		}
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			out = append(out, v)
		}
	}
	return out
}
