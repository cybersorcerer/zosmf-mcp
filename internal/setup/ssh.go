package setup

import "strings"

// SSHFields is the catalog for the ZOAU tools, which reach z/OS over SSH
// rather than through z/OSMF.
func SSHFields() []Field {
	return []Field{
		{
			ID: "ssh-host", YAMLKey: "host", Label: "SSH host",
			Help: "The LPAR's SSH address. Usually the host part of the z/OSMF URL.",
			Kind: KindString, Cardinality: One, Required: true,
			Example: "zos.example.com", Group: "zoau",
		},
		{
			ID: "ssh-port", YAMLKey: "port", Label: "SSH port",
			Help: "Defaults to 22.", Kind: KindInt, Cardinality: One,
			Default: "22", Group: "zoau",
		},
		{
			ID: "ssh-user", YAMLKey: "username", Label: "SSH user",
			Help: "The z/OS user the ZOAU commands run as.",
			Kind: KindString, Cardinality: One, Required: true,
			Example: "ibmuser", Group: "zoau",
		},
		{
			ID: "ssh-key-file", YAMLKey: "key_file", Label: "SSH private key",
			Help: "Only key authentication is supported. In a container this is the path inside it, e.g. /keys/id_rsa.",
			Kind: KindString, Cardinality: One, Required: true,
			Example: "~/.ssh/id_rsa", Group: "zoau",
		},
		{
			ID: "zoau-path", YAMLKey: "", Label: "ZOAU installation path",
			Help: "ZOAU_HOME, PATH and LIBPATH are derived from it.",
			Kind: KindString, Cardinality: One, Required: true,
			Example: "/usr/lpp/IBM/zoautil", Group: "zoau",
		},
	}
}

// ZOAUEnvironment turns the installation path into the three variables
// RunSSH exports before every command.
func ZOAUEnvironment(installPath string) []Entry {
	p := strings.TrimRight(installPath, "/")
	return []Entry{{Key: "environment", Value: map[string]string{
		"ZOAU_HOME": p,
		"PATH":      p + "/bin:/bin:/usr/bin",
		"LIBPATH":   p + "/lib:/lib:/usr/lib",
	}}}
}
