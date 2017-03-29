L1181
initLXC()
// Prepare all the paths
			srcPath, exist := m["source"]
			if !exist {
				srcPath = m["path"]
			}
			relativeSrcPath := strings.TrimPrefix(srcPath, "/")
			devName := fmt.Sprintf("unix.%s", strings.Replace(relativeSrcPath, "/", "-", -1))
			devPath := filepath.Join(c.DevicesPath(), devName)
			tgtPath, exist := m["path"]
			if !exist {
				tgtPath = m["source"]
			}
			relativeTgtPath := strings.TrimPrefix(tgtPath, "/")

			// Set the bind-mount entry
			err = lxcSetConfigItem(cc, "lxc.mount.entry", fmt.Sprintf("%s %s none bind,create=file", devPath, relativeTgtPath))
			if err != nil {
				return err
			}
      
      
  L1444
  startCommon()
  case "unix-char", "unix-block":
			source, exist := m["source"]
			if !exist {
				source = m["path"]
			}
			if m["path"] != "" && m["major"] == "" && m["minor"] == "" && !shared.PathExists(source) {
				return "", fmt.Errorf("Missing source '%s' for device '%s'", m["path"], name)
			}
		}
    
   L4489
   createUnixDevice()
   // Our device paths
	srcPath, exist := m["source"]
	if !exist {
		srcPath = m["path"]
	}
	relativeSrcPath := strings.TrimPrefix(srcPath, "/")
	devName := fmt.Sprintf("unix.%s", strings.Replace(relativeSrcPath, "/", "-", -1))
	devPath := filepath.Join(c.DevicesPath(), devName)
  
  L4631
 insertUnixDevice()

/ Bind-mount it into the container
	tgtPath, exist := m["path"]
	if !exist {
		tgtPath = m["source"]
	}
	relativeTgtPath := strings.TrimSuffix(tgtPath, "/")
	err = c.insertMount(devPath, relativeTgtPath, "none", syscall.MS_BIND)
	if err != nil {
		return fmt.Errorf("Failed to add mount for device: %s", err)
	}
  
  
  L4691
  removeUnixDevice()
  // Figure out the paths
  srcPath, exist := m["source"]
	if !exist {
		srcPath = m["path"]
	}
	relativeSrcPath := strings.TrimPrefix(srcPath, "/")
	devName := fmt.Sprintf("unix.%s", strings.Replace(relativeSrcPath, "/", "-", -1))
	devPath := filepath.Join(c.DevicesPath(), devName)
  
  
  // Remove the bind-mount from the container
	tgtPath, exist := m ["path"]
	if !exist {
		tgtPath = m["source"]
	}
	relativeTgtPath := strings.TrimPrefix(tgtPath, "/")
	if c.FileExists(relativeTgtPath) == nil {
		err = c.removeMount(tgtPath)
		if err != nil {
			return fmt.Errorf("Error unmounting the device: %s", err)
		}

		err = c.FileRemove(relativeTgtPath)
		if err != nil {
			return fmt.Errorf("Error removing the device: %s", err)
		}
	}
