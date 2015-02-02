package softlayer

import (
	"errors"
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/packer"
	"log"
	"os"
	"time"
)

// The unique ID for this builder.
const BuilderId = "packer.softlayer"

type config struct {
	common.PackerConfig `mapstructure:",squash"`

	Username         string `mapstructure:"username"`
	APIKey           string `mapstructure:"api_key"`
	DatacenterName   string `mapstructure:"datacenter_name"`
	ImageName        string `mapstructure:"image_name"`
	ImageDescription string `mapstructure:"image_description"`
	ImageType        string `mapstructure:"image_type"`
	BaseImageId      string `mapstructure:"base_image_id"`
	BaseOsCode       string `mapstructure:"base_os_code"`

	InstanceName         string `mapstructure:"instance_name"`
	InstanceDomain       string `mapstructure:"instance_domain"`
	InstanceCpu          int    `mapstructure:"instance_cpu"`
	InstanceMemory       int64  `mapstructure:"instance_memory"`
	InstanceNetworkSpeed int    `mapstructure:"instance_network_speed"`
	InstanceDiskCapacity int    `mapstructure:"instance_disk_capacity"`
	SshPort              int64  `mapstructure:"ssh_port"`
	SshUserName          string `mapstructure:"ssh_username"`
	SshPrivateKeyFile    string `mapstructure:"ssh_private_key_file"`

	RawSshTimeout   string `mapstructure:"ssh_timeout"`
	RawStateTimeout string `mapstructure:"instance_state_timeout"`

	SshTimeout   time.Duration
	StateTimeout time.Duration

	tpl *packer.ConfigTemplate
}

// Image Types
const IMAGE_TYPE_FLEX     = "flex"
const IMAGE_TYPE_STANDARD = "standard"

// Builder represents a Packer Builder.
type Builder struct {
	config config
	runner multistep.Runner
}

// Prepare processes the build configuration parameters.
func (self *Builder) Prepare(raws ...interface{}) (parms []string, retErr error) {
	metadata, err := common.DecodeConfig(&self.config, raws...)
	if err != nil {
		return nil, err
	}

	// Check that there aren't any unknown configuration keys defined
	errs := common.CheckUnusedConfig(metadata)
	if errs == nil {
		errs = &packer.MultiError{}
	}

	self.config.tpl, err = packer.NewConfigTemplate()
	if err != nil {
		return nil, err
	}
	self.config.tpl.UserVars = self.config.PackerUserVars

	// Assign default values if possible
	if self.config.APIKey == "" {
		// Default to environment variable for api_key, if it exists
		self.config.APIKey = os.Getenv("SOFTLAYER_API_KEY")
	}

	if self.config.Username == "" {
		// Default to environment variable for client_id, if it exists
		self.config.Username = os.Getenv("SOFTLAYER_USER_NAME")
	}

	if self.config.DatacenterName == "" {
		self.config.DatacenterName = "ams01"
	}

	if self.config.InstanceName == "" {
		self.config.InstanceName = fmt.Sprintf("packer-softlayer-%s", time.Now().Unix())
	}

	if self.config.InstanceDomain == "" {
		self.config.InstanceDomain = "defaultdomain.com"
	}

	if self.config.ImageDescription == "" {
		self.config.ImageDescription = "Instance snapshot. Generated by packer.io"
	}

	if self.config.ImageType == "" {
		self.config.ImageType = IMAGE_TYPE_FLEX
	}

	if self.config.InstanceCpu == 0 {
		self.config.InstanceCpu = 1
	}

	if self.config.InstanceMemory == 0 {
		self.config.InstanceMemory = 1024
	}

	if self.config.InstanceNetworkSpeed == 0 {
		self.config.InstanceNetworkSpeed = 10
	}

	if self.config.InstanceDiskCapacity == 0 {
		self.config.InstanceDiskCapacity = 25
	}

	if self.config.SshPort == 0 {
		self.config.SshPort = 22
	}

	if self.config.SshUserName == "" {
		self.config.SshUserName = "root"
	}

	if self.config.RawSshTimeout == "" {
		self.config.RawSshTimeout = "5m"
	}

	if self.config.RawStateTimeout == "" {
		self.config.RawStateTimeout = "10m"
	}

	templates := map[string]*string{
		"username":               &self.config.Username,
		"api_key":                &self.config.APIKey,
		"datacenter_name":        &self.config.DatacenterName,
		"base_image_id":          &self.config.BaseImageId,
		"image_name":             &self.config.ImageName,
		"image_description":      &self.config.ImageDescription,
		"image_type":             &self.config.ImageType,
		"base_os_code":           &self.config.BaseOsCode,
		"instance_name":          &self.config.InstanceName,
		"instance_domain":        &self.config.InstanceDomain,
		"ssh_timeout":            &self.config.RawSshTimeout,
		"instance_state_timeout": &self.config.RawStateTimeout,
		"ssh_username":           &self.config.SshUserName,
		"ssh_private_key_file":   &self.config.SshPrivateKeyFile,
	}

	for n, ptr := range templates {
		var err error
		*ptr, err = self.config.tpl.Process(*ptr, nil)
		if err != nil {
			errs = packer.MultiErrorAppend(errs, fmt.Errorf("Error processing %s: %s", n, err))
		}
	}

	// Check for required configurations that will display errors if not set
	if self.config.APIKey == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("api_key or the SOFTLAYER_API_KEY environment variable must be specified"))
	}

	if self.config.Username == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("username or the SOFTLAYER_USER_NAME environment variable must be specified"))
	}

	if self.config.ImageName == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("image_name must be specified"))
	}

	if self.config.ImageType != IMAGE_TYPE_FLEX && self.config.ImageType != IMAGE_TYPE_STANDARD {
		errs = packer.MultiErrorAppend(
			errs, fmt.Errorf("Unknown image_type '%s'. Must be one of 'flex' (the default) or 'standard'.", self.config.ImageType))
	}

	if self.config.BaseImageId == "" && self.config.BaseOsCode == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("please specify base_image_id or base_os_code"))
	}

	if self.config.BaseImageId != "" && self.config.BaseOsCode != "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("please specify only one of base_image_id or base_os_code"))
	}

	if self.config.BaseImageId != "" && self.config.SshPrivateKeyFile == "" {
		errs = packer.MultiErrorAppend(
			errs, errors.New("when using base_image_id, you must specify ssh_private_key_file "+
				"since automatic ssh key config for custom images isn't supported by SoftLayer API"))
	}

	// Translate date configuration data from string to time format
	sshTimeout, err := time.ParseDuration(self.config.RawSshTimeout)
	if err != nil {
		errs = packer.MultiErrorAppend(
			errs, fmt.Errorf("Failed parsing ssh_timeout: %s", err))
	}
	self.config.SshTimeout = sshTimeout

	stateTimeout, err := time.ParseDuration(self.config.RawStateTimeout)
	if err != nil {
		errs = packer.MultiErrorAppend(
			errs, fmt.Errorf("Failed parsing state_timeout: %s", err))
	}
	self.config.StateTimeout = stateTimeout

	log.Println(common.ScrubConfig(self.config, self.config.APIKey, self.config.Username))

	if len(errs.Errors) > 0 {
		retErr = errors.New(errs.Error())
	}

	return nil, retErr
}

// Run executes a SoftLayer Packer build and returns a packer.Artifact
// representing a SoftLayer machine image (flex).
func (self *Builder) Run(ui packer.Ui, hook packer.Hook, cache packer.Cache) (packer.Artifact, error) {

	// Create the client
	client := SoftlayerClient{}.New(self.config.Username, self.config.APIKey)

	// Set up the state which is used to share state between the steps
	state := new(multistep.BasicStateBag)
	state.Put("config", self.config)
	state.Put("client", client)
	state.Put("hook", hook)
	state.Put("ui", ui)

	// Build the steps
	steps := []multistep.Step{
		&stepCreateSshKey{
			PrivateKeyFile: self.config.SshPrivateKeyFile,
		},
		new(stepCreateInstance),
		new(stepWaitforInstance),
		&common.StepConnectSSH{
			SSHAddress:     sshAddress,
			SSHConfig:      sshConfig,
			SSHWaitTimeout: self.config.SshTimeout,
		},
		new(common.StepProvision),
		new(stepCaptureImage),
	}

	// Create the runner which will run the steps we just build
	self.runner = &multistep.BasicRunner{Steps: steps}
	self.runner.Run(state)

	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	if _, ok := state.GetOk("image_id"); !ok {
		log.Println("Failed to find image_id in state. Bug?")
		return nil, nil
	}

	// Create an artifact and return it
	artifact := &Artifact{
		imageName:      self.config.ImageName,
		imageId:        state.Get("image_id").(string),
		datacenterName: self.config.DatacenterName,
		client:         client,
	}

	return artifact, nil
}

// Cancel.
func (self *Builder) Cancel() {
	if self.runner != nil {
		log.Println("Cancelling the step runner...")
		self.runner.Cancel()
	}
	fmt.Println("Cancelling the builder")
}
