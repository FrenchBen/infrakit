package kubernetes

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	//@TODO
	// Generate SSL for Cluster
	// Generate root CA
	// system("mkdir -p ssl && ./../../lib/init-ssl-ca ssl") or abort ("failed generating SSL artifacts")
	if err := execScript("ssl/init-ssl-ca", k.SslDir); err != nil {
		return instance, err
	}

	// Generate creds with admin/kube-admin
	// Generate admin key/cert
	// system("./../../lib/init-ssl ssl admin kube-admin") or abort("failed generating admin SSL artifacts")
	if err := execScript("ssl/init-ssl", k.SslDir, "admin", "kube-admin"); err != nil {
		return instance, err
	}

	// Generate kubeconfig file in tutorial folder
	logicalID := string(*instance.LogicalID)
	sslTar, err := provisionMachineSSL(k, "apiserver", "kube-apiserver-"+logicalID, []string{logicalID, "10.3.0.1"})

	var properties map[string]interface{}

	if err := json.Unmarshal(*instance.Properties, &properties); err != nil {
		return instance, err
	}
	properties["SSL"] = sslTar
	data, err := json.Marshal(properties)
	instance.Properties = data

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
	tarFile := fmt.Sprintf("ssl/%s.tar", cn)
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
	data, err := Asset("ssl/init-ssl")
	if err != nil {
		// Asset was not found.
		log.Errorf("Script error: %v", err)
		return err
	}
	cmd := exec.Command("bash", "-s", strings.Join(args, ", "))
	cmd.Stdin = strings.NewReader(string(data))
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		log.Errorf("Error in bash script: %v", err)
		return err
	}
	fmt.Printf("Output: %q\n", out.String())
	return nil
}

// def provisionMachineSSL(machine,certBaseName,cn,ipAddrs)
//   tarFile = "ssl/#{cn}.tar"
//   ipString = ipAddrs.map.with_index { |ip, i| "IP.#{i+1}=#{ip}"}.join(",")
//   system("./../../lib/init-ssl ssl #{certBaseName} #{cn} #{ipString}") or abort("failed generating #{cn} SSL artifacts")
//   machine.vm.provision :file, :source => tarFile, :destination => "/tmp/ssl.tar"
//   machine.vm.provision :shell, :inline => "mkdir -p /etc/kubernetes/ssl && tar -C /etc/kubernetes/ssl -xf /tmp/ssl.tar", :privileged => true
// end
