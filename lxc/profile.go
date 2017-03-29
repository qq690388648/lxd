package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"syscall"

	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v2"

	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
	"github.com/lxc/lxd/shared/i18n"
	"github.com/lxc/lxd/shared/termios"
)

type profileCmd struct {
}

func (c *profileCmd) showByDefault() bool {
	return true
}

func (c *profileCmd) profileEditHelp() string {
	return i18n.G(
		`### This is a yaml representation of the profile.
### Any line starting with a '# will be ignored.
###
### A profile consists of a set of configuration items followed by a set of
### devices.
###
### An example would look like:
### name: onenic
### config:
###   raw.lxc: lxc.aa_profile=unconfined
### devices:
###   eth0:
###     nictype: bridged
###     parent: lxdbr0
###     type: nic
###
### Note that the name is shown but cannot be changed`)
}

func (c *profileCmd) usage() string {
	return i18n.G(
		`Usage: lxc profile <subcommand> [options]

Manage container configuration profiles.

*Profile configuration*
lxc profile list [<remote>:]
    List available profiles.

lxc profile show [<remote>:]<profile>
    Show details of a profile.

lxc profile create [<remote>:]<profile>
    Create a profile.

lxc profile copy [<remote>:]<profile> [<remote>:]<profile>
    Copy the profile.

lxc profile get [<remote>:]<profile> <key>
    Get profile configuration.

lxc profile set [<remote>:]<profile> <key> <value>
    Set profile configuration.

lxc profile unset [<remote>:]<profile> <key>
    Unset profile configuration.

lxc profile delete [<remote>:]<profile>
    Delete a profile.

lxc profile edit [<remote>:]<profile>
    Edit profile, either by launching external editor or reading STDIN.

*Profile assignment*
lxc profile assign [<remote>:]<container> <profiles>
    Replace the current set of profiles for the container by the one provided.

lxc profile add [<remote>:]<container> <profile>
    Add a profile to a container

lxc profile remove [<remote>:]<container> <profile>
    Remove the profile from a container

*Device management*
lxc profile device list [<remote>:]<profile>
    List devices in the given profile.

lxc profile device show [<remote>:]<profile>
    Show full device details in the given profile.

lxc profile device remove [<remote>:]<profile> <name>
    Remove a device from a profile.

lxc profile device get [<remote>:]<profile> <name> <key>
    Get a device property.

lxc profile device set [<remote>:]<profile> <name> <key> <value>
    Set a device property.

lxc profile device unset [<remote>:]<profile> <name> <key>
    Unset a device property.

lxc profile device add [<remote>:]<profile> <device> <type> [key=value...]
    Add a profile device, such as a disk or a nic, to the containers using the specified profile.

*Examples*
cat profile.yaml | lxc profile edit <profile>
    Update a profile using the content of profile.yaml

lxc profile assign foo default,bar
    Set the profiles for "foo" to "default" and "bar".

lxc profile assign foo default
    Reset "foo" to only using the "default" profile.

lxc profile assign foo ''
    Remove all profile from "foo"`)
}

func (c *profileCmd) flags() {}

func (c *profileCmd) run(config *lxd.Config, args []string) error {
	if len(args) < 1 {
		return errUsage
	}

	if args[0] == "list" {
		return c.doProfileList(config, args)
	}

	if len(args) < 2 {
		return errArgs
	}

	remote, profile := config.ParseRemoteAndContainer(args[1])
	client, err := lxd.NewClient(config, remote)
	if err != nil {
		return err
	}

	switch args[0] {
	case "create":
		return c.doProfileCreate(client, profile)
	case "delete":
		return c.doProfileDelete(client, profile)
	case "device":
		return c.doProfileDevice(config, args)
	case "edit":
		return c.doProfileEdit(client, profile)
	case "apply", "assign":
		container := profile
		switch len(args) {
		case 2:
			profile = ""
		case 3:
			profile = args[2]
		default:
			return errArgs
		}
		return c.doProfileAssign(client, container, profile)
	case "add":
		container := profile
		switch len(args) {
		case 2:
			profile = ""
		case 3:
			profile = args[2]
		default:
			return errArgs
		}
		return c.doProfileAdd(client, container, profile)
	case "remove":
		container := profile
		switch len(args) {
		case 2:
			profile = ""
		case 3:
			profile = args[2]
		default:
			return errArgs
		}
		return c.doProfileRemove(client, container, profile)
	case "get":
		return c.doProfileGet(client, profile, args[2:])
	case "set":
		return c.doProfileSet(client, profile, args[2:])
	case "unset":
		return c.doProfileUnset(client, profile, args[2:])
	case "copy":
		return c.doProfileCopy(config, client, profile, args[2:])
	case "show":
		return c.doProfileShow(client, profile)
	default:
		return errArgs
	}
}

func (c *profileCmd) doProfileCreate(client *lxd.Client, p string) error {
	err := client.ProfileCreate(p)
	if err == nil {
		fmt.Printf(i18n.G("Profile %s created")+"\n", p)
	}
	return err
}

func (c *profileCmd) doProfileEdit(client *lxd.Client, p string) error {
	// If stdin isn't a terminal, read text from it
	if !termios.IsTerminal(int(syscall.Stdin)) {
		contents, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := api.ProfilePut{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}
		return client.PutProfile(p, newdata)
	}

	// Extract the current value
	profile, err := client.ProfileConfig(p)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(&profile)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := shared.TextEditor("", []byte(c.profileEditHelp()+"\n\n"+string(data)))
	if err != nil {
		return err
	}

	for {
		// Parse the text received from the editor
		newdata := api.ProfilePut{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = client.PutProfile(p, newdata)
		}

		// Respawn the editor
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.G("Config parsing error: %s")+"\n", err)
			fmt.Println(i18n.G("Press enter to open the editor again"))

			_, err := os.Stdin.Read(make([]byte, 1))
			if err != nil {
				return err
			}

			content, err = shared.TextEditor("", content)
			if err != nil {
				return err
			}
			continue
		}
		break
	}
	return nil
}

func (c *profileCmd) doProfileDelete(client *lxd.Client, p string) error {
	err := client.ProfileDelete(p)
	if err == nil {
		fmt.Printf(i18n.G("Profile %s deleted")+"\n", p)
	}
	return err
}

func (c *profileCmd) doProfileAssign(client *lxd.Client, d string, p string) error {
	resp, err := client.AssignProfile(d, p)
	if err != nil {
		return err
	}

	err = client.WaitForSuccess(resp.Operation)
	if err == nil {
		if p == "" {
			p = i18n.G("(none)")
		}
		fmt.Printf(i18n.G("Profiles %s applied to %s")+"\n", p, d)
	}

	return err
}

func (c *profileCmd) doProfileAdd(client *lxd.Client, d string, p string) error {
	ct, err := client.ContainerInfo(d)
	if err != nil {
		return err
	}

	ct.Profiles = append(ct.Profiles, p)

	err = client.UpdateContainerConfig(d, ct.Writable())
	if err != nil {
		return err
	}

	fmt.Printf(i18n.G("Profile %s added to %s")+"\n", p, d)

	return err
}

func (c *profileCmd) doProfileRemove(client *lxd.Client, d string, p string) error {
	ct, err := client.ContainerInfo(d)
	if err != nil {
		return err
	}

	if !shared.StringInSlice(p, ct.Profiles) {
		return fmt.Errorf("Profile %s isn't currently applied to %s", p, d)
	}

	profiles := []string{}
	for _, profile := range ct.Profiles {
		if profile == p {
			continue
		}

		profiles = append(profiles, profile)
	}

	ct.Profiles = profiles

	err = client.UpdateContainerConfig(d, ct.Writable())
	if err != nil {
		return err
	}

	fmt.Printf(i18n.G("Profile %s removed from %s")+"\n", p, d)

	return err
}

func (c *profileCmd) doProfileShow(client *lxd.Client, p string) error {
	profile, err := client.ProfileConfig(p)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(&profile)
	if err != nil {
		return err
	}

	fmt.Printf("%s", data)

	return nil
}

func (c *profileCmd) doProfileCopy(config *lxd.Config, client *lxd.Client, p string, args []string) error {
	if len(args) != 1 {
		return errArgs
	}
	remote, newname := config.ParseRemoteAndContainer(args[0])
	if newname == "" {
		newname = p
	}

	dest, err := lxd.NewClient(config, remote)
	if err != nil {
		return err
	}

	return client.ProfileCopy(p, newname, dest)
}

func (c *profileCmd) doProfileDevice(config *lxd.Config, args []string) error {
	// device add b1 eth0 nic type=bridged
	// device list b1
	// device remove b1 eth0
	if len(args) < 3 {
		return errArgs
	}

	cfg := configCmd{}

	switch args[1] {
	case "add":
		return cfg.deviceAdd(config, "profile", args)
	case "remove":
		return cfg.deviceRm(config, "profile", args)
	case "list":
		return cfg.deviceList(config, "profile", args)
	case "show":
		return cfg.deviceShow(config, "profile", args)
	case "get":
		return cfg.deviceGet(config, "profile", args)
	case "set":
		return cfg.deviceSet(config, "profile", args)
	case "unset":
		return cfg.deviceUnset(config, "profile", args)
	default:
		return errArgs
	}
}

func (c *profileCmd) doProfileGet(client *lxd.Client, p string, args []string) error {
	// we shifted @args so so it should read "<key>"
	if len(args) != 1 {
		return errArgs
	}

	resp, err := client.GetProfileConfig(p)
	if err != nil {
		return err
	}
	for k, v := range resp {
		if k == args[0] {
			fmt.Printf("%s\n", v)
		}
	}
	return nil
}

func (c *profileCmd) doProfileSet(client *lxd.Client, p string, args []string) error {
	// we shifted @args so so it should read "<key> [<value>]"
	if len(args) < 1 {
		return errArgs
	}

	key := args[0]
	var value string
	if len(args) < 2 {
		value = ""
	} else {
		value = args[1]
	}

	if !termios.IsTerminal(int(syscall.Stdin)) && value == "-" {
		buf, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("Can't read from stdin: %s", err)
		}
		value = string(buf[:])
	}

	err := client.SetProfileConfigItem(p, key, value)
	return err
}

func (c *profileCmd) doProfileUnset(client *lxd.Client, p string, args []string) error {
	// we shifted @args so so it should read "<key> [<value>]"
	if len(args) != 1 {
		return errArgs
	}

	return c.doProfileSet(client, p, args)
}

func (c *profileCmd) doProfileList(config *lxd.Config, args []string) error {
	var remote string
	if len(args) > 1 {
		var name string
		remote, name = config.ParseRemoteAndContainer(args[1])
		if name != "" {
			return fmt.Errorf(i18n.G("Cannot provide container name to list"))
		}
	} else {
		remote = config.DefaultRemote
	}

	client, err := lxd.NewClient(config, remote)
	if err != nil {
		return err
	}

	profiles, err := client.ListProfiles()
	if err != nil {
		return err
	}

	data := [][]string{}
	for _, profile := range profiles {
		strUsedBy := fmt.Sprintf("%d", len(profile.UsedBy))
		data = append(data, []string{profile.Name, strUsedBy})
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetRowLine(true)
	table.SetHeader([]string{
		i18n.G("NAME"),
		i18n.G("USED BY")})
	sort.Sort(byName(data))
	table.AppendBulk(data)
	table.Render()

	return nil
}
