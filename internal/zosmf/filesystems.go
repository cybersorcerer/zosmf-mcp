package zosmf

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Filesystem represents a mounted z/OS filesystem.
type Filesystem struct {
	Name       string   `json:"name"`
	Mountpoint string   `json:"mountpoint"`
	FSTName    string   `json:"fstname"`
	Status     string   `json:"status"`
	Mode       []string `json:"mode"`
	Dev        int      `json:"dev"`
	SysName    string   `json:"sysname"`
	ReadIBC    int      `json:"readibc"`
	WriteIBC   int      `json:"writeibc"`
	DirIBC     int      `json:"diribc"`
}

// CreateZFSRequest contains parameters for creating a new zFS filesystem.
type CreateZFSRequest struct {
	CylsPri         int      `json:"cylsPri"`
	CylsSec         int      `json:"cylsSec,omitempty"`
	Owner           string   `json:"owner,omitempty"`
	Group           string   `json:"group,omitempty"`
	Perms           int      `json:"perms,omitempty"`
	StorageClass    string   `json:"storageClass,omitempty"`
	ManagementClass string   `json:"managementClass,omitempty"`
	DataClass       string   `json:"dataClass,omitempty"`
	Volumes         []string `json:"volumes,omitempty"`
}

// ListFilesystems lists all mounted filesystems, optionally filtered by name or path.
func (c *Client) ListFilesystems(fsname, path string) ([]Filesystem, error) {
	query := url.Values{}
	if fsname != "" {
		query.Set("fsname", fsname)
	}
	if path != "" {
		query.Set("path", path)
	}

	apiPath := "/zosmf/restfiles/mfs"
	if len(query) > 0 {
		apiPath += "?" + query.Encode()
	}

	body, err := c.Get(apiPath)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Items []Filesystem `json:"items"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing filesystems response: %w", err)
	}

	return resp.Items, nil
}

// MountFilesystem mounts a filesystem at the given mount point.
func (c *Client) MountFilesystem(fsname, mountPoint, fsType, mode string) error {
	path := fmt.Sprintf("/zosmf/restfiles/mfs/%s", url.PathEscape(fsname))

	reqBody := map[string]string{
		"action":      "mount",
		"mount-point": mountPoint,
		"fs-type":     fsType,
		"mode":        mode,
	}

	_, err := c.Put(path, reqBody)
	return err
}

// UnmountFilesystem unmounts a filesystem.
func (c *Client) UnmountFilesystem(fsname string) error {
	path := fmt.Sprintf("/zosmf/restfiles/mfs/%s", url.PathEscape(fsname))

	reqBody := map[string]string{
		"action": "unmount",
	}

	_, err := c.Put(path, reqBody)
	return err
}

// CreateZFS creates a new zFS filesystem.
func (c *Client) CreateZFS(zfsname string, req CreateZFSRequest) error {
	path := fmt.Sprintf("/zosmf/restfiles/mfs/zfs/%s", url.PathEscape(zfsname))

	_, err := c.Post(path, req)
	return err
}

// DeleteZFS deletes a zFS filesystem.
func (c *Client) DeleteZFS(zfsname string) error {
	path := fmt.Sprintf("/zosmf/restfiles/mfs/zfs/%s", url.PathEscape(zfsname))

	_, err := c.Delete(path)
	return err
}
