package native

import (
	"fmt"
	"log"

	"kernel.org/pub/linux/libs/security/libcap/cap"
)

func Drop(c cap.Value) (*cap.Set, error) {
	current, err := cap.GetPID(0) // 0 = current process
	if err != nil {
		return nil, fmt.Errorf("get caps: %w", err)
	}

	// log.Println(current.String())

	// build a reduced set with the cap dropped from E+I+P
	reduced, err := current.Dup()
	if err != nil {
		return nil, fmt.Errorf("dup caps: %w", err)
	}
	if err := reduced.SetFlag(cap.Effective, false, c); err != nil {
		return nil, fmt.Errorf("drop effective cap: %w", err)
	}
	if err := reduced.SetFlag(cap.Inheritable, false, c); err != nil {
		return nil, fmt.Errorf("drop inheritable cap: %w", err)
	}
	if err := reduced.SetFlag(cap.Permitted, false, c); err != nil {
		return nil, fmt.Errorf("drop permitted cap: %w", err)
	}

	// apply the reduced set
	if err := reduced.SetProc(); err != nil {
		return nil, fmt.Errorf("set reduced caps: %w", err)
	}

	return current, nil
}

func Restore(original *cap.Set) {
	// restore original caps
	if err := original.SetProc(); err != nil {
		// Restoration failed — log loudly, do not silently swallow
		log.Fatalf("FATAL: failed to restore capabilities: %v", err)
	}
}

func dropAndRestoreCap(c cap.Value, fn func() error) error {
	// snapshot current capability set
	current, err := cap.GetPID(0) // 0 = current process
	if err != nil {
		return fmt.Errorf("get caps: %w", err)
	}

	// build a reduced set with the cap dropped from E+I+P
	reduced, err := current.Dup()
	if err != nil {
		return fmt.Errorf("dup caps: %w", err)
	}
	if err := reduced.SetFlag(cap.Effective, false, c); err != nil {
		return err
	}
	if err := reduced.SetFlag(cap.Inheritable, false, c); err != nil {
		return err
	}
	if err := reduced.SetFlag(cap.Permitted, false, c); err != nil {
		return err
	}

	// apply the reduced set ---
	if err := reduced.SetProc(); err != nil {
		return fmt.Errorf("set reduced caps: %w", err)
	}

	// run the sensitive operation ---
	fnErr := fn()

	// restore original caps (always, even if fn failed) ---
	if err := current.SetProc(); err != nil {
		// Restoration failed — log loudly, do not silently swallow
		log.Fatalf("FATAL: failed to restore capabilities: %v", err)
	}

	return fnErr
}
