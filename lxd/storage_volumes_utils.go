package main

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/version"
)

const (
	storagePoolVolumeTypeContainer = iota
	storagePoolVolumeTypeImage
	storagePoolVolumeTypeCustom
)

// Leave the string type in here! This guarantees that go treats this is as a
// typed string constant. Removing it causes go to treat these as untyped string
// constants which is not what we want.
const (
	storagePoolVolumeTypeNameContainer string = "container"
	storagePoolVolumeTypeNameImage     string = "image"
	storagePoolVolumeTypeNameCustom    string = "custom"
)

// Leave the string type in here! This guarantees that go treats this is as a
// typed string constant. Removing it causes go to treat these as untyped string
// constants which is not what we want.
const (
	storagePoolVolumeApiEndpointContainers string = "containers"
	storagePoolVolumeApiEndpointImages     string = "images"
	storagePoolVolumeApiEndpointCustom     string = "custom"
)

var supportedVolumeTypes = []int{storagePoolVolumeTypeContainer, storagePoolVolumeTypeImage, storagePoolVolumeTypeCustom}

func storagePoolVolumeTypeNameToType(volumeTypeName string) (int, error) {
	switch volumeTypeName {
	case storagePoolVolumeTypeNameContainer:
		return storagePoolVolumeTypeContainer, nil
	case storagePoolVolumeTypeNameImage:
		return storagePoolVolumeTypeImage, nil
	case storagePoolVolumeTypeNameCustom:
		return storagePoolVolumeTypeCustom, nil
	}

	return -1, fmt.Errorf("Invalid storage volume type name.")
}

func storagePoolVolumeTypeNameToApiEndpoint(volumeTypeName string) (string, error) {
	switch volumeTypeName {
	case storagePoolVolumeTypeNameContainer:
		return storagePoolVolumeApiEndpointContainers, nil
	case storagePoolVolumeTypeNameImage:
		return storagePoolVolumeApiEndpointImages, nil
	case storagePoolVolumeTypeNameCustom:
		return storagePoolVolumeApiEndpointCustom, nil
	}

	return "", fmt.Errorf("Invalid storage volume type name.")
}

func storagePoolVolumeTypeToName(volumeType int) (string, error) {
	switch volumeType {
	case storagePoolVolumeTypeContainer:
		return storagePoolVolumeTypeNameContainer, nil
	case storagePoolVolumeTypeImage:
		return storagePoolVolumeTypeNameImage, nil
	case storagePoolVolumeTypeCustom:
		return storagePoolVolumeTypeNameCustom, nil
	}

	return "", fmt.Errorf("Invalid storage volume type.")
}

func storagePoolVolumeTypeToApiEndpoint(volumeType int) (string, error) {
	switch volumeType {
	case storagePoolVolumeTypeContainer:
		return storagePoolVolumeApiEndpointContainers, nil
	case storagePoolVolumeTypeImage:
		return storagePoolVolumeApiEndpointImages, nil
	case storagePoolVolumeTypeCustom:
		return storagePoolVolumeApiEndpointCustom, nil
	}

	return "", fmt.Errorf("Invalid storage volume type.")
}

func storagePoolVolumeUpdate(d *Daemon, poolName string, volumeName string, volumeType int, newConfig map[string]string) error {
	s, err := storagePoolVolumeInit(d, poolName, volumeName, volumeType)
	if err != nil {
		return err
	}

	oldWritable := s.GetStoragePoolVolumeWritable()
	newWritable := oldWritable

	// Backup the current state
	oldConfig := map[string]string{}
	err = shared.DeepCopy(&oldWritable.Config, &oldConfig)
	if err != nil {
		return err
	}

	// Define a function which reverts everything.  Defer this function
	// so that it doesn't need to be explicitly called in every failing
	// return path. Track whether or not we want to undo the changes
	// using a closure.
	undoChanges := true
	defer func() {
		if undoChanges {
			s.SetStoragePoolVolumeWritable(&oldWritable)
		}
	}()

	// Diff the configurations
	changedConfig := []string{}
	userOnly := true
	for key := range oldConfig {
		if oldConfig[key] != newConfig[key] {
			if !strings.HasPrefix(key, "user.") {
				userOnly = false
			}

			if !shared.StringInSlice(key, changedConfig) {
				changedConfig = append(changedConfig, key)
			}
		}
	}

	for key := range newConfig {
		if oldConfig[key] != newConfig[key] {
			if !strings.HasPrefix(key, "user.") {
				userOnly = false
			}

			if !shared.StringInSlice(key, changedConfig) {
				changedConfig = append(changedConfig, key)
			}
		}
	}

	// Skip on no change
	if len(changedConfig) == 0 {
		return nil
	}

	// Update the storage pool
	if !userOnly {
		err = s.StoragePoolVolumeUpdate(changedConfig)
		if err != nil {
			return err
		}
	}

	newWritable.Config = newConfig

	// Apply the new configuration
	s.SetStoragePoolVolumeWritable(&newWritable)

	poolID, err := dbStoragePoolGetID(d.db, poolName)
	if err != nil {
		return err
	}

	// Update the database
	err = dbStoragePoolVolumeUpdate(d.db, volumeName, volumeType, poolID, newConfig)
	if err != nil {
		return err
	}

	// Success, update the closure to mark that the changes should be kept.
	undoChanges = false

	return nil
}

func storagePoolVolumeUsedByGet(d *Daemon, volumeName string, volumeTypeName string) ([]string, error) {
	// Look for containers using the interface
	cts, err := dbContainersList(d.db, cTypeRegular)
	if err != nil {
		return []string{}, err
	}

	volumeUsedBy := []string{}
	volumeNameWithType := fmt.Sprintf("%s/%s", volumeTypeName, volumeName)
	for _, ct := range cts {
		c, err := containerLoadByName(d, ct)
		if err != nil {
			continue
		}

		for _, d := range c.LocalDevices() {
			if d["type"] != "disk" {
				continue
			}

			// Make sure that we don't compare against stuff like
			// "container////bla" but only against "container/bla".
			cleanSource := filepath.Clean(d["source"])
			if cleanSource == volumeName || cleanSource == volumeNameWithType {
				volumeUsedBy = append(volumeUsedBy, fmt.Sprintf("/%s/containers/%s", version.APIVersion, ct))
			}
		}
	}

	profiles, err := profilesUsingPoolVolumeGetNames(d.db, volumeName, volumeTypeName)
	if err != nil {
		return []string{}, err
	}

	if len(volumeUsedBy) == 0 && len(profiles) == 0 {
		return []string{}, err
	}

	for _, pName := range profiles {
		volumeUsedBy = append(volumeUsedBy, fmt.Sprintf("/%s/profiles/%s", version.APIVersion, pName))
	}

	return volumeUsedBy, nil
}

func profilesUsingPoolVolumeGetNames(db *sql.DB, volumeName string, volumeType string) ([]string, error) {
	usedBy := []string{}

	profiles, err := dbProfiles(db)
	if err != nil {
		return usedBy, err
	}

	for _, pName := range profiles {
		_, profile, err := dbProfileGet(db, pName)
		if err != nil {
			return usedBy, err
		}

		volumeNameWithType := fmt.Sprintf("%s/%s", volumeType, volumeName)
		for _, v := range profile.Devices {
			if v["type"] != "disk" {
				continue
			}

			// Can't be a storage volume.
			if filepath.IsAbs(v["source"]) {
				continue
			}

			// Make sure that we don't compare against stuff
			// like "container////bla" but only against
			// "container/bla".
			cleanSource := filepath.Clean(v["source"])
			if cleanSource == volumeName || cleanSource == volumeNameWithType {
				usedBy = append(usedBy, pName)
			}
		}
	}

	return usedBy, nil
}

func storagePoolVolumeDBCreate(d *Daemon, poolName string, volumeName string, volumeTypeName string, volumeConfig map[string]string) error {
	// Check that the name of the new storage volume is valid. (For example.
	// zfs pools cannot contain "/" in their names.)
	err := storageValidName(volumeName)
	if err != nil {
		return err
	}

	// Convert the volume type name to our internal integer representation.
	volumeType, err := storagePoolVolumeTypeNameToType(volumeTypeName)
	if err != nil {
		return err
	}

	// We currently only allow to create storage volumes of type
	// storagePoolVolumeTypeCustom. So check, that nothing else was
	// requested.
	if volumeType != storagePoolVolumeTypeCustom {
		return fmt.Errorf("Currently not allowed to create storage volumes of type %s.", volumeTypeName)
	}

	// Load storage pool the volume will be attached to.
	poolID, poolStruct, err := dbStoragePoolGet(d.db, poolName)
	if err != nil {
		return err
	}

	// Check that a storage volume of the same storage volume type does not
	// already exist.
	volumeID, _ := dbStoragePoolVolumeGetTypeID(d.db, volumeName, volumeType, poolID)
	if volumeID > 0 {
		return fmt.Errorf("A storage volume of type %s does already exist.", volumeTypeName)
	}

	// Make sure that we don't pass a nil to the next function.
	if volumeConfig == nil {
		volumeConfig = map[string]string{}
	}

	// Validate the requested storage volume configuration.
	err = storageVolumeValidateConfig(poolName, volumeConfig, poolStruct)
	if err != nil {
		return err
	}

	err = storageVolumeFillDefault(poolName, volumeConfig, poolStruct)
	if err != nil {
		return err
	}

	// Create the database entry for the storage volume.
	_, err = dbStoragePoolVolumeCreate(d.db, volumeName, volumeType, poolID, volumeConfig)
	if err != nil {
		return fmt.Errorf("Error inserting %s of type %s into database: %s", poolName, volumeTypeName, err)
	}

	return nil
}

func storagePoolVolumeCreateInternal(d *Daemon, poolName string, volumeName string, volumeTypeName string, volumeConfig map[string]string) error {
	err := storagePoolVolumeDBCreate(d, poolName, volumeName, volumeTypeName, volumeConfig)
	if err != nil {
		return err
	}

	// Convert the volume type name to our internal integer representation.
	volumeType, err := storagePoolVolumeTypeNameToType(volumeTypeName)
	if err != nil {
		return err
	}

	s, err := storagePoolVolumeInit(d, poolName, volumeName, volumeType)
	if err != nil {
		return err
	}

	poolID, _ := s.GetContainerPoolInfo()

	// Create storage volume.
	err = s.StoragePoolVolumeCreate()
	if err != nil {
		dbStoragePoolVolumeDelete(d.db, volumeName, volumeType, poolID)
		return err
	}

	return nil
}
