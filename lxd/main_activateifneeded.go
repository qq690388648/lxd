package main

import (
	"fmt"
	"os"

	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

func cmdActivateIfNeeded() error {
	// Only root should run this
	if os.Geteuid() != 0 {
		return fmt.Errorf("This must be run as root")
	}

	// Don't start a full daemon, we just need DB access
	d := &Daemon{
		lxcpath: shared.VarPath("containers"),
	}

	if !shared.PathExists(shared.VarPath("lxd.db")) {
		shared.LogDebugf("No DB, so no need to start the daemon now.")
		return nil
	}

	err := initializeDbObject(d, shared.VarPath("lxd.db"))
	if err != nil {
		return err
	}

	/* Load all config values from the database */
	err = daemonConfigInit(d.db)
	if err != nil {
		return err
	}

	// Look for network socket
	value := daemonConfig["core.https_address"].Get()
	if value != "" {
		shared.LogDebugf("Daemon has core.https_address set, activating...")
		_, err := lxd.NewClient(&lxd.DefaultConfig, "local")
		return err
	}

	// Load the idmap for unprivileged containers
	d.IdmapSet, err = shared.DefaultIdmapSet()
	if err != nil {
		return err
	}

	// Look for auto-started or previously started containers
	result, err := dbContainersList(d.db, cTypeRegular)
	if err != nil {
		return err
	}

	for _, name := range result {
		c, err := containerLoadByName(d, name)
		if err != nil {
			return err
		}

		config := c.ExpandedConfig()
		lastState := config["volatile.last_state.power"]
		autoStart := config["boot.autostart"]

		if c.IsRunning() {
			shared.LogDebugf("Daemon has running containers, activating...")
			_, err := lxd.NewClient(&lxd.DefaultConfig, "local")
			return err
		}

		if lastState == "RUNNING" || lastState == "Running" || shared.IsTrue(autoStart) {
			shared.LogDebugf("Daemon has auto-started containers, activating...")
			_, err := lxd.NewClient(&lxd.DefaultConfig, "local")
			return err
		}
	}

	shared.LogDebugf("No need to start the daemon now.")
	return nil
}
