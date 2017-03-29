package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

func cmdInit() error {
	var defaultPrivileged int // controls whether we set security.privileged=true
	var storageSetup bool     // == supportedStoragePoolDrivers
	var storageBackend string // == supportedStoragePoolDrivers
	var storageLoopSize int64 // Size in GB
	var storageDevice string  // Path
	var storagePool string    // pool name
	var storageDataset string // existing ZFS pool name
	var networkAddress string // Address
	var networkPort int64     // Port
	var trustPassword string  // Trust password
	var imagesAutoUpdate bool // controls whether we set images.auto_update_interval to 0
	var bridgeName string     // Bridge name
	var bridgeIPv4 string     // IPv4 address
	var bridgeIPv4Nat bool    // IPv4 address
	var bridgeIPv6 string     // IPv6 address
	var bridgeIPv6Nat bool    // IPv6 address

	// Detect userns
	defaultPrivileged = -1
	runningInUserns = shared.RunningInUserNS()
	imagesAutoUpdate = true

	backendsAvailable := []string{"dir"}

	// Check available backends
	for _, driver := range supportedStoragePoolDrivers {
		if driver == "dir" {
			continue
		}

		// btrfs can work in user namespaces too. (If
		// source=/some/path/on/btrfs is used.)
		if runningInUserns && driver != "btrfs" {
			continue
		}

		// Initialize a core storage interface for the given driver.
		_, err := storageCoreInit(driver)
		if err != nil {
			continue
		}

		backendsAvailable = append(backendsAvailable, driver)
	}

	reader := bufio.NewReader(os.Stdin)

	askBool := func(question string, default_ string) bool {
		for {
			fmt.Printf(question)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSuffix(input, "\n")
			if input == "" {
				input = default_
			}
			if shared.StringInSlice(strings.ToLower(input), []string{"yes", "y"}) {
				return true
			} else if shared.StringInSlice(strings.ToLower(input), []string{"no", "n"}) {
				return false
			}

			fmt.Printf("Invalid input, try again.\n\n")
		}
	}

	askChoice := func(question string, choices []string, default_ string) string {
		for {
			fmt.Printf(question)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSuffix(input, "\n")
			if input == "" {
				input = default_
			}
			if shared.StringInSlice(input, choices) {
				return input
			}

			fmt.Printf("Invalid input, try again.\n\n")
		}
	}

	askInt := func(question string, min int64, max int64, default_ string) int64 {
		for {
			fmt.Printf(question)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSuffix(input, "\n")
			if input == "" {
				input = default_
			}
			intInput, err := strconv.ParseInt(input, 10, 64)

			if err == nil && (min == -1 || intInput >= min) && (max == -1 || intInput <= max) {
				return intInput
			}

			fmt.Printf("Invalid input, try again.\n\n")
		}
	}

	askString := func(question string, default_ string, validate func(string) error) string {
		for {
			fmt.Printf(question)
			input, _ := reader.ReadString('\n')
			input = strings.TrimSuffix(input, "\n")
			if input == "" {
				input = default_
			}
			if validate != nil {
				result := validate(input)
				if result != nil {
					fmt.Printf("Invalid input: %s\n\n", result)
					continue
				}
			}
			if len(input) != 0 {
				return input
			}

			fmt.Printf("Invalid input, try again.\n\n")
		}
	}

	askPassword := func(question string) string {
		for {
			fmt.Printf(question)
			pwd, _ := terminal.ReadPassword(0)
			fmt.Printf("\n")
			inFirst := string(pwd)
			inFirst = strings.TrimSuffix(inFirst, "\n")

			fmt.Printf("Again: ")
			pwd, _ = terminal.ReadPassword(0)
			fmt.Printf("\n")
			inSecond := string(pwd)
			inSecond = strings.TrimSuffix(inSecond, "\n")

			if inFirst == inSecond {
				return inFirst
			}

			fmt.Printf("Invalid input, try again.\n\n")
		}
	}

	// Confirm that LXD is online
	c, err := lxd.NewClient(&lxd.DefaultConfig, "local")
	if err != nil {
		return fmt.Errorf("Unable to talk to LXD: %s", err)
	}

	pools, err := c.ListStoragePools()
	if err != nil {
		// We should consider this fatal since this means
		// something's wrong with the daemon.
		return err
	}

	if *argAuto {
		if *argStorageBackend == "" {
			*argStorageBackend = "dir"
		}

		// Do a bunch of sanity checks
		if !shared.StringInSlice(*argStorageBackend, supportedStoragePoolDrivers) {
			return fmt.Errorf("The requested backend '%s' isn't supported by lxd init.", *argStorageBackend)
		}

		if !shared.StringInSlice(*argStorageBackend, backendsAvailable) {
			return fmt.Errorf("The requested backend '%s' isn't available on your system (missing tools).", *argStorageBackend)
		}

		if *argStorageBackend == "dir" {
			if *argStorageCreateLoop != -1 || *argStorageCreateDevice != "" || *argStorageDataset != "" {
				return fmt.Errorf("None of --storage-pool, --storage-create-device or --storage-create-loop may be used with the 'dir' backend.")
			}
		} else {
			if *argStorageCreateLoop != -1 && *argStorageCreateDevice != "" {
				return fmt.Errorf("Only one of --storage-create-device or --storage-create-loop can be specified.")
			}
		}

		if *argNetworkAddress == "" {
			if *argNetworkPort != -1 {
				return fmt.Errorf("--network-port cannot be used without --network-address.")
			}
			if *argTrustPassword != "" {
				return fmt.Errorf("--trust-password cannot be used without --network-address.")
			}
		}

		storageBackend = *argStorageBackend
		storageLoopSize = *argStorageCreateLoop
		storageDevice = *argStorageCreateDevice
		storageDataset = *argStorageDataset
		networkAddress = *argNetworkAddress
		networkPort = *argNetworkPort
		trustPassword = *argTrustPassword
		storagePool = "default"

		// FIXME: Allow to configure multiple storage pools on auto init
		// run if explicit arguments to do so are passed.
		if len(pools) == 0 {
			storageSetup = true
		}
	} else {
		if *argStorageBackend != "" || *argStorageCreateDevice != "" || *argStorageCreateLoop != -1 || *argStorageDataset != "" || *argNetworkAddress != "" || *argNetworkPort != -1 || *argTrustPassword != "" {
			return fmt.Errorf("Init configuration is only valid with --auto")
		}

		defaultStorage := "dir"
		if shared.StringInSlice("zfs", backendsAvailable) {
			defaultStorage = "zfs"
		}

		// User chose an already existing storage pool name. Ask him
		// again if he still wants to create one.
	askForStorageAgain:
		storageSetup = askBool("Do you want to configure a new storage pool (yes/no) [default=yes]? ", "yes")
		if storageSetup {
			storagePool = askString("Name of the new storage pool [default=default]: ", "default", nil)
			_, err := c.StoragePoolGet(storagePool)
			if err == nil {
				fmt.Printf("The requested storage pool \"%s\" already exists. Please choose another name.\n", storagePool)
				// Ask the user again if hew wants to create a
				// storage pool.
				goto askForStorageAgain
			}

			storageBackend = askChoice(fmt.Sprintf("Name of the storage backend to use (%s) [default=%s]: ", strings.Join(backendsAvailable, ", "), defaultStorage), supportedStoragePoolDrivers, defaultStorage)

			if !shared.StringInSlice(storageBackend, supportedStoragePoolDrivers) {
				return fmt.Errorf("The requested backend '%s' isn't supported by lxd init.", storageBackend)
			}

			if !shared.StringInSlice(storageBackend, backendsAvailable) {
				return fmt.Errorf("The requested backend '%s' isn't available on your system (missing tools).", storageBackend)
			}
		}

		if storageSetup && storageBackend != "dir" {
			storageLoopSize = -1
			q := fmt.Sprintf("Create a new %s pool (yes/no) [default=yes]? ", strings.ToUpper(storageBackend))
			if askBool(q, "yes") {
				if askBool("Would you like to use an existing block device (yes/no) [default=no]? ", "no") {
					deviceExists := func(path string) error {
						if !shared.IsBlockdevPath(path) {
							return fmt.Errorf("'%s' is not a block device", path)
						}
						return nil
					}
					storageDevice = askString("Path to the existing block device: ", "", deviceExists)
				} else {
					st := syscall.Statfs_t{}
					err := syscall.Statfs(shared.VarPath(), &st)
					if err != nil {
						return fmt.Errorf("couldn't statfs %s: %s", shared.VarPath(), err)
					}

					/* choose 15 GB < x < 100GB, where x is 20% of the disk size */
					def := uint64(st.Frsize) * st.Blocks / (1024 * 1024 * 1024) / 5
					if def > 100 {
						def = 100
					}
					if def < 15 {
						def = 15
					}

					q := fmt.Sprintf("Size in GB of the new loop device (1GB minimum) [default=%dGB]: ", def)
					storageLoopSize = askInt(q, 1, -1, fmt.Sprintf("%d", def))
				}
			} else {
				q := fmt.Sprintf("Name of the existing %s pool or dataset: ", strings.ToUpper(storageBackend))
				storageDataset = askString(q, "", nil)
			}
		}

		// Detect lack of uid/gid
		needPrivileged := false
		idmapset, err := shared.DefaultIdmapSet()
		if err != nil || len(idmapset.Idmap) == 0 || idmapset.Usable() != nil {
			needPrivileged = true
		}

		if runningInUserns && needPrivileged {
			fmt.Printf(`
We detected that you are running inside an unprivileged container.
This means that unless you manually configured your host otherwise,
you will not have enough uid and gid to allocate to your containers.

LXD can re-use your container's own allocation to avoid the problem.
Doing so makes your nested containers slightly less safe as they could
in theory attack their parent container and gain more privileges than
they otherwise would.

`)
			if askBool("Would you like to have your containers share their parent's allocation (yes/no) [default=yes]? ", "yes") {
				defaultPrivileged = 1
			} else {
				defaultPrivileged = 0
			}
		}

		if askBool("Would you like LXD to be available over the network (yes/no) [default=no]? ", "no") {
			isIPAddress := func(s string) error {
				if s != "all" && net.ParseIP(s) == nil {
					return fmt.Errorf("'%s' is not an IP address", s)
				}
				return nil
			}

			networkAddress = askString("Address to bind LXD to (not including port) [default=all]: ", "all", isIPAddress)
			if networkAddress == "all" {
				networkAddress = "::"
			}

			if net.ParseIP(networkAddress).To4() == nil {
				networkAddress = fmt.Sprintf("[%s]", networkAddress)
			}
			networkPort = askInt("Port to bind LXD to [default=8443]: ", 1, 65535, "8443")
			trustPassword = askPassword("Trust password for new clients: ")
		}

		if !askBool("Would you like stale cached images to be updated automatically (yes/no) [default=yes]? ", "yes") {
			imagesAutoUpdate = false
		}

	askForNetworkAgain:
		bridgeName = ""
		if askBool("Would you like to create a new network bridge (yes/no) [default=yes]? ", "yes") {
			bridgeName = askString("What should the new bridge be called [default=lxdbr0]? ", "lxdbr0", networkValidName)
			_, err := c.NetworkGet(bridgeName)
			if err == nil {
				fmt.Printf("The requested network bridge \"%s\" already exists. Please choose another name.\n", bridgeName)
				// Ask the user again if hew wants to create a
				// storage pool.
				goto askForNetworkAgain
			}

			bridgeIPv4 = askString("What IPv4 address should be used (CIDR subnet notation, “auto” or “none”) [default=auto]? ", "auto", func(value string) error {
				if shared.StringInSlice(value, []string{"auto", "none"}) {
					return nil
				}
				return networkValidAddressCIDRV4(value)
			})

			if !shared.StringInSlice(bridgeIPv4, []string{"auto", "none"}) {
				bridgeIPv4Nat = askBool("Would you like LXD to NAT IPv4 traffic on your bridge? [default=yes]? ", "yes")
			}

			bridgeIPv6 = askString("What IPv6 address should be used (CIDR subnet notation, “auto” or “none”) [default=auto]? ", "auto", func(value string) error {
				if shared.StringInSlice(value, []string{"auto", "none"}) {
					return nil
				}
				return networkValidAddressCIDRV6(value)
			})

			if !shared.StringInSlice(bridgeIPv6, []string{"auto", "none"}) {
				bridgeIPv6Nat = askBool("Would you like LXD to NAT IPv6 traffic on your bridge? [default=yes]? ", "yes")
			}
		}
	}

	if storageSetup {
		// Unset core.https_address and core.trust_password
		for _, key := range []string{"core.https_address", "core.trust_password"} {
			_, err = c.SetServerConfig(key, "")
			if err != nil {
				return err
			}
		}

		// Pool configuration
		storagePoolConfig := map[string]string{}

		if storageDevice != "" {
			storagePoolConfig["source"] = storageDevice
			if storageDataset != "" {
				storagePool = storageDataset
			}
		} else if storageLoopSize != -1 {
			if storageDataset != "" {
				storagePool = storageDataset
			}
		} else {
			storagePoolConfig["source"] = storageDataset
		}

		if storageLoopSize > 0 {
			storagePoolConfig["size"] = strconv.FormatInt(storageLoopSize, 10) + "GB"
		}

		// Create the requested storage pool.
		err := c.StoragePoolCreate(storagePool, storageBackend, storagePoolConfig)
		if err != nil {
			return err
		}

		// When lxd init is rerun and there are already storage pools
		// configured, do not try to set a root disk device in the
		// default profile again. Let the user figure this out.
		if len(pools) == 0 {
			// Check if there even is a default profile.
			profiles, err := c.ListProfiles()
			if err != nil {
				return err
			}

			defaultProfileExists := false
			for _, p := range profiles {
				if p.Name != "default" {
					continue
				}

				defaultProfileExists = true

				foundRootDiskDevice := false
				for k, v := range p.Devices {
					if v["path"] == "/" && v["source"] == "" {
						foundRootDiskDevice = true

						// Unconditionally overwrite because if the user ends up
						// with a clean LXD but with a pool property key existing in
						// the default profile it must be empty otherwise it would
						// not have been possible to delete the storage pool in
						// the first place.
						p.ProfilePut.Devices[k]["pool"] = storagePool

						// Update profile devices.
						err := c.PutProfile("default", p.ProfilePut)
						if err != nil {
							return err
						}
						shared.LogDebugf("Set pool property of existing root disk device \"%s\" in profile \"default\" to \"%s\".", storagePool)

						break
					}
				}

				if foundRootDiskDevice {
					break
				}

				props := []string{"path=/", fmt.Sprintf("pool=%s", storagePool)}
				_, err = c.ProfileDeviceAdd("default", "root", "disk", props)
				if err != nil {
					return err
				}
				break
			}

			if !defaultProfileExists {
				shared.LogWarnf("Did not find profile \"default\" so no default storage pool will be set. Manual intervention needed.")
			}
		}
	}

	if defaultPrivileged == 0 {
		err = c.SetProfileConfigItem("default", "security.privileged", "")
		if err != nil {
			return err
		}
	} else if defaultPrivileged == 1 {
		err = c.SetProfileConfigItem("default", "security.privileged", "true")
		if err != nil {
		}
	}

	if imagesAutoUpdate {
		ss, err := c.ServerStatus()
		if err != nil {
			return err
		}
		if val, ok := ss.Config["images.auto_update_interval"]; ok && val == "0" {
			_, err = c.SetServerConfig("images.auto_update_interval", "")
			if err != nil {
				return err
			}
		}
	} else {
		_, err = c.SetServerConfig("images.auto_update_interval", "0")
		if err != nil {
			return err
		}
	}

	if networkAddress != "" {
		_, err = c.SetServerConfig("core.https_address", fmt.Sprintf("%s:%d", networkAddress, networkPort))
		if err != nil {
			return err
		}

		if trustPassword != "" {
			_, err = c.SetServerConfig("core.trust_password", trustPassword)
			if err != nil {
				return err
			}
		}
	}

	if bridgeName != "" {
		bridgeConfig := map[string]string{}
		bridgeConfig["ipv4.address"] = bridgeIPv4
		bridgeConfig["ipv6.address"] = bridgeIPv6

		if bridgeIPv4Nat {
			bridgeConfig["ipv4.nat"] = "true"
		}

		if bridgeIPv6Nat {
			bridgeConfig["ipv6.nat"] = "true"
		}

		err = c.NetworkCreate(bridgeName, bridgeConfig)
		if err != nil {
			return err
		}

		props := []string{"nictype=bridged", fmt.Sprintf("parent=%s", bridgeName)}
		_, err = c.ProfileDeviceAdd("default", "eth0", "nic", props)
		if err != nil {
			return err
		}
	}

	fmt.Printf("LXD has been successfully configured.\n")
	return nil
}
