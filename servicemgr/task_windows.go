//go:build windows

package servicemgr

import (
	"context"
	"fmt"
	"os/user"
)

// Windows backend: a Task Scheduler task with a logon trigger, run as the current
// user with the interactive token and least privilege (no elevation).

// Register creates the task, or updates it when the same owner already has it.
func Register(ctx context.Context, s Spec) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cur, err := user.Current()
	if err != nil {
		return fmt.Errorf("servicemgr: current user: %w", err)
	}
	xmlText, err := buildTaskXML(s, cur.Username)
	if err != nil {
		return err
	}
	existing, found, err := readScheduledTask(s.Name)
	if err != nil {
		return err
	}
	if found {
		st, err := parseTaskXML(existing.XML)
		if err != nil {
			return err
		}
		if err := checkOwner(s.Name, st.Owner, s.Owner); err != nil {
			return err
		}
	}
	return registerScheduledTask(s.Name, xmlText, cur.Username, found)
}

// owned loads the task and verifies the ownership marker; it is the shared
// precondition of Unregister and Start.
func owned(ctx context.Context, name, owner string) error {
	if err := validateRef(name, owner); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, found, err := readScheduledTask(name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrNotInstalled, name)
	}
	st, err := parseTaskXML(existing.XML)
	if err != nil {
		return err
	}
	return checkOwner(name, st.Owner, owner)
}

// Unregister deletes the task after the ownership check.
func Unregister(ctx context.Context, name, owner string) error {
	if err := owned(ctx, name, owner); err != nil {
		return err
	}
	if err := deleteScheduledTask(name); err != nil {
		return fmt.Errorf("servicemgr: delete task %q: %w", name, err)
	}
	return nil
}

// Start runs the task once now (IRegisteredTask.Run) after the ownership check.
func Start(ctx context.Context, name, owner string) error {
	if err := owned(ctx, name, owner); err != nil {
		return err
	}
	if err := runScheduledTask(name); err != nil {
		return fmt.Errorf("servicemgr: run task %q: %w", name, err)
	}
	return nil
}

// Query reports the task registration; a missing task is Installed=false, nil.
func Query(ctx context.Context, name string) (Status, error) {
	if err := validateName(name); err != nil {
		return Status{}, err
	}
	if err := ctx.Err(); err != nil {
		return Status{}, err
	}
	existing, found, err := readScheduledTask(name)
	if err != nil || !found {
		return Status{}, err
	}
	return parseTaskXML(existing.XML)
}
