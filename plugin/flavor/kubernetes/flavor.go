package kubernetes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/infrakit/plugin/group/types"
	"github.com/docker/infrakit/spi/flavor"
	"github.com/docker/infrakit/spi/instance"
)

// Spec is the model of the Properties section of the top level group spec.
type Spec struct {
	// Init
	Init []string
	Role string

	// Tags
	Tags map[string]string
}

// NewPlugin creates a Flavor plugin that doesn't do very much. It assumes instances are
// identical (cattles) but can assume specific identities (via the LogicalIDs).  The
// instances here are treated identically because we have constant Init that applies
// to all instances
func NewPlugin(sslDir string) flavor.Plugin {
	return &kubernetesFlavor{Index: 0, SslDir: sslDir}
}

type kubernetesFlavor struct {
	Index  int
	SslDir string
}

func (k kubernetesFlavor) Validate(flavorProperties json.RawMessage, allocation types.AllocationMethod) error {
	return json.Unmarshal(flavorProperties, &Spec{})
}

func (k kubernetesFlavor) Healthy(flavorProperties json.RawMessage, inst instance.Description) (flavor.Health, error) {
	// TODO: We could add support for shell code in the Spec for a command to run for checking health.
	return flavor.Healthy, nil
}

func (k kubernetesFlavor) Drain(flavorProperties json.RawMessage, inst instance.Description) error {
	// TODO: We could add support for shell code in the Spec for a drain command to run.
	return nil
}

func (k kubernetesFlavor) Prepare(
	flavor json.RawMessage,
	instance instance.Spec,
	allocation types.AllocationMethod) (instance.Spec, error) {

	s := Spec{}
	err := json.Unmarshal(flavor, &s)
	if err != nil {
		return instance, err
	}

	// Append Init
	lines := []string{}
	if instance.Init != "" {
		lines = append(lines, instance.Init)
	}
	lines = append(lines, s.Init...)

	instance.Init = strings.Join(lines, "\n")
	if _, err := os.Stat(k.SslDir + "/kube-admin.tar"); os.IsNotExist(err) {
		log.Errorf("SSL Directory does not exist: %v", k.SslDir)
		return instance, err
	}

	// Only create the admin SSL if not exist:
	if _, err := os.Stat(k.SslDir + "/kube-admin.tar"); os.IsNotExist(err) {
		// Generate root CA
		if err := execScript("ssl/init-ssl-ca", k.SslDir); err != nil {
			return instance, err
		}
		// Generate admin key/cert
		if err := execScript("ssl/init-ssl", k.SslDir, "admin", "kube-admin"); err != nil {
			return instance, err
		}
	}

	// Generate kubeconfig file in tutorial folder
	logicalID := string(*instance.LogicalID)
	ipAddrs := []string{logicalID}
	if s.Role == "controller" {
		ipAddrs = append(ipAddrs, "10.3.0.1")
	}
	sslTar, err := provisionMachineSSL(k, "apiserver", "kube-apiserver-"+logicalID, ipAddrs)

	var properties map[string]interface{}

	if err := json.Unmarshal(*instance.Properties, &properties); err != nil {
		return instance, err
	}
	properties["SSL"] = sslTar
	data, err := json.Marshal(properties)
	raw := json.RawMessage(string(data))
	instance.Properties = &raw

	// Append tags
	for k, v := range s.Tags {
		if instance.Tags == nil {
			instance.Tags = map[string]string{}
		}
		instance.Tags[k] = v
	}
	return instance, nil
}

func provisionMachineSSL(k kubernetesFlavor, certBaseName string, cn string, ipAddrs []string) (string, error) {
	tarFile := fmt.Sprintf("%s/%s.tar", k.SslDir, cn)
	ipString := ""
	for i, ip := range ipAddrs {
		ipString = ipString + fmt.Sprintf("IP.%d=%s,", i+1, ip)
	}
	if err := execScript("ssl/init-ssl", k.SslDir, certBaseName, cn, ipString); err != nil {
		return "", err
	}
	return tarFile, nil
}

func execScript(script string, args ...string) error {
	data, err := Asset(script)
	if err != nil {
		// Asset was not found.
		log.Errorf("Script error: %v", err)
		return err
	}
	args = append([]string{"-s"}, args...)
	cmd := exec.Command("bash", args...)
	cmd.Stdin = strings.NewReader(string(data))
	var out, outErr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &outErr
	err = cmd.Run()
	log.Debugf("Output: %q\n", out.String())
	if err != nil {
		log.Errorf("Error in bash script: %v", err)
		log.Errorf("Stderr: %v", outErr.String())
		return err
	}
	return nil
}
