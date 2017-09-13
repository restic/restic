package main

// To run this package...
// go run gen.go -- --sdk 3.14.16

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	do "gopkg.in/godo.v2"
)

type service struct {
	Name      string
	Fullname  string
	Namespace string
	TaskName  string
	Tag       string
	Input     string
	Output    string
}

const (
	testsSubDir = "tests"
)

type mapping struct {
	PlaneInput  string
	PlaneOutput string
	Services    []service
}

var (
	gopath          = os.Getenv("GOPATH")
	sdkVersion      string
	autorestDir     string
	swaggersDir     string
	testGen         bool
	deps            do.S
	services        = []*service{}
	servicesMapping = []mapping{
		{
			PlaneOutput: "arm",
			PlaneInput:  "resource-manager",
			Services: []service{
				{Name: "advisor"},
				{Name: "analysisservices"},
				// {
				// Autorest Bug
				// Name: "apimanagement",
				// },
				{Name: "appinsights"},
				{Name: "authorization"},
				{Name: "automation"},
				{Name: "batch"},
				{Name: "billing"},
				{Name: "cdn"},
				// {
				// bug in AutoRest (duplicated files)
				// Name: "cognitiveservices",
				// },
				{Name: "commerce"},
				{Name: "compute"},
				{
					Name:  "containerservice",
					Input: "compute",
					Tag:   "package-container-service-2017-01",
				},
				{Name: "consumption"},
				{Name: "containerinstance"},
				{Name: "containerregistry"},
				{Name: "cosmos-db"},
				{Name: "customer-insights"},
				{
					Name:   "account",
					Input:  "datalake-analytics",
					Output: "datalake-analytics/account",
				},
				{
					Name:   "account",
					Input:  "datalake-store",
					Output: "datalake-store/account",
				},
				{Name: "devtestlabs"},
				{Name: "dns"},
				{Name: "eventgrid"},
				{Name: "eventhub"},
				{Name: "hdinsight"},
				{Name: "intune"},
				{Name: "iothub"},
				{Name: "keyvault"},
				{Name: "logic"},
				{
					Name:   "commitmentplans",
					Input:  "machinelearning",
					Output: "machinelearning/commitmentPlans",
					Tag:    "package-commitmentPlans-2016-05-preview",
				},
				{
					Name:   "webservices",
					Input:  "machinelearning",
					Output: "machinelearning/webservices",
					Tag:    "package-webservices-2017-01",
				},
				{Name: "mediaservices"},
				{Name: "mobileengagement"},
				{Name: "monitor"},
				{Name: "mysql"},
				{Name: "network"},
				{Name: "notificationhubs"},
				// {
				// bug in the Go generator https://github.com/Azure/autorest/issues/2219
				// Name: "operationalinsights",
				// },
				{Name: "postgresql"},
				{Name: "powerbiembedded"},
				{Name: "recoveryservices"},
				{Name: "recoveryservicesbackup"},
				{Name: "recoveryservicessiterecovery"},
				{Name: "redis"},
				{Name: "relay"},
				{Name: "resourcehealth"},
				{
					Name:   "features",
					Input:  "resources",
					Output: "resources/features",
					Tag:    "package-features-2015-12",
				},
				{
					Name:   "links",
					Input:  "resources",
					Output: "resources/links",
					Tag:    "package-links-2016-09",
				},
				{
					Name:   "locks",
					Input:  "resources",
					Output: "resources/locks",
					Tag:    "package-locks-2016-09",
				},
				{
					Name:   "managedapplications",
					Input:  "resources",
					Output: "resources/managedapplications",
					Tag:    "package-managedapplications-2016-09",
				},
				{
					Name:   "policy",
					Input:  "resources",
					Output: "resources/policy",
					Tag:    "package-policy-2016-12",
				},
				{
					Name:   "resources",
					Input:  "resources",
					Output: "resources/resources",
					Tag:    "package-resources-2017-05",
				},
				{
					Name:   "subscriptions",
					Input:  "resources",
					Output: "resources/subscriptions",
					Tag:    "package-subscriptions-2016-06",
				},
				{Name: "scheduler"},
				{Name: "search"},
				{Name: "servermanagement"},
				{Name: "service-map"},
				{Name: "servicebus"},
				{Name: "servicefabric"},
				{Name: "sql"},
				{Name: "storage"},
				{Name: "storageimportexport"},
				{Name: "storsimple8000series"},
				{Name: "streamanalytics"},
				// {
				// error in the modeler
				// 	Name:    "timeseriesinsights",
				// },
				{Name: "trafficmanager"},
				{Name: "visualstudio"},
				{Name: "web"},
			},
		},
		{
			PlaneOutput: "dataplane",
			PlaneInput:  "data-plane",
			Services: []service{
				{
					Name: "keyvault",
				},
			},
		},
		{
			PlaneInput: "data-plane",
			Services: []service{
				{
					Name:   "filesystem",
					Input:  "datalake-store",
					Output: "datalake-store/filesystem",
				},
			},
		},
		{
			PlaneOutput: "arm",
			PlaneInput:  "data-plane",
			Services: []service{
				{
					Name: "graphrbac",
				},
			},
		},
	}
)

func main() {
	for _, swaggerGroup := range servicesMapping {
		swg := swaggerGroup
		for _, service := range swg.Services {
			s := service
			initAndAddService(&s, swg.PlaneInput, swg.PlaneOutput)
		}
	}
	do.Godo(tasks)
}

func initAndAddService(service *service, planeInput, planeOutput string) {
	if service.Input == "" {
		service.Input = service.Name
	}
	service.Input = filepath.Join(service.Input, planeInput, "readme.md")
	if service.Output == "" {
		service.Output = service.Name
	}
	service.TaskName = fmt.Sprintf("%s>%s", planeOutput, strings.Join(strings.Split(service.Output, "/"), ">"))
	service.Fullname = filepath.Join(planeOutput, service.Output)
	service.Namespace = filepath.Join("github.com", "Azure", "azure-sdk-for-go", service.Fullname)
	service.Output = filepath.Join(gopath, "src", service.Namespace)

	services = append(services, service)
	deps = append(deps, service.TaskName)
}

func tasks(p *do.Project) {
	p.Task("default", do.S{"setvars", "generate:all", "management"}, nil)
	p.Task("setvars", nil, setVars)
	p.Use("generate", generateTasks)
	p.Use("gofmt", formatTasks)
	p.Use("gobuild", buildTasks)
	p.Use("golint", lintTasks)
	p.Use("govet", vetTasks)
	p.Task("management", do.S{"setvars"}, managementVersion)
	p.Task("addVersion", nil, addVersion)
}

func setVars(c *do.Context) {
	if gopath == "" {
		panic("Gopath not set\n")
	}

	sdkVersion = c.Args.MustString("s", "sdk", "version")
	autorestDir = c.Args.MayString("", "a", "ar", "autorest")
	swaggersDir = c.Args.MayString("", "w", "sw", "swagger")
	testGen = c.Args.MayBool(false, "t", "testgen")
}

func generateTasks(p *do.Project) {
	addTasks(generate, p)
}

func generate(service *service) {
	codegen := "--go"
	if testGen {
		codegen = "--go.testgen"
		service.Fullname = strings.Join([]string{service.Fullname, testsSubDir}, string(os.PathSeparator))
		service.Output = filepath.Join(service.Output, testsSubDir)
	}

	fmt.Printf("Generating %s...\n\n", service.Fullname)

	fullInput := ""
	if swaggersDir == "" {
		fullInput = fmt.Sprintf("https://raw.githubusercontent.com/Azure/azure-rest-api-specs/current/specification/%s", service.Input)
	} else {
		fullInput = filepath.Join(swaggersDir, "azure-rest-api-specs", "specification", service.Input)
	}

	execCommand := "autorest"
	commandArgs := []string{
		fullInput,
		codegen,
		"--license-header=MICROSOFT_APACHE",
		fmt.Sprintf("--namespace=%s", service.Name),
		fmt.Sprintf("--output-folder=%s", service.Output),
		fmt.Sprintf("--package-version=%s", sdkVersion),
		"--clear-output-folder",
		"--can-clear-output-folder",
	}
	if service.Tag != "" {
		commandArgs = append(commandArgs, fmt.Sprintf("--tag=%s", service.Tag))
	}
	if testGen {
		commandArgs = append([]string{"-LEGACY"}, commandArgs...)
	}

	if autorestDir != "" {
		// if an AutoRest directory was specified then assume
		// the caller wants to use a locally-built version.
		commandArgs = append(commandArgs, fmt.Sprintf("--use=%s", autorestDir))
	}

	autorest := exec.Command(execCommand, commandArgs...)

	fmt.Println(commandArgs)

	if _, err := runner(autorest); err != nil {
		panic(fmt.Errorf("Autorest error: %s", err))
	}

	format(service)
	build(service)
	lint(service)
	vet(service)
}

func formatTasks(p *do.Project) {
	addTasks(format, p)
}

func format(service *service) {
	fmt.Printf("Formatting %s...\n\n", service.Fullname)
	gofmt := exec.Command("gofmt", "-w", service.Output)
	_, err := runner(gofmt)
	if err != nil {
		panic(fmt.Errorf("gofmt error: %s", err))
	}
}

func buildTasks(p *do.Project) {
	addTasks(build, p)
}

func build(service *service) {
	fmt.Printf("Building %s...\n\n", service.Fullname)
	gobuild := exec.Command("go", "build", service.Namespace)
	_, err := runner(gobuild)
	if err != nil {
		panic(fmt.Errorf("go build error: %s", err))
	}
}

func lintTasks(p *do.Project) {
	addTasks(lint, p)
}

func lint(service *service) {
	fmt.Printf("Linting %s...\n\n", service.Fullname)
	golint := exec.Command(filepath.Join(gopath, "bin", "golint"), service.Namespace)
	_, err := runner(golint)
	if err != nil {
		panic(fmt.Errorf("golint error: %s", err))
	}
}

func vetTasks(p *do.Project) {
	addTasks(vet, p)
}

func vet(service *service) {
	fmt.Printf("Vetting %s...\n\n", service.Fullname)
	govet := exec.Command("go", "vet", service.Namespace)
	_, err := runner(govet)
	if err != nil {
		panic(fmt.Errorf("go vet error: %s", err))
	}
}

func addVersion(c *do.Context) {
	gitStatus := exec.Command("git", "status", "-s")
	out, err := runner(gitStatus)
	if err != nil {
		panic(fmt.Errorf("Git error: %s", err))
	}
	files := strings.Split(out, "\n")

	for _, f := range files {
		if strings.HasPrefix(f, " M ") && strings.HasSuffix(f, "version.go") {
			gitAdd := exec.Command("git", "add", f[3:])
			_, err := runner(gitAdd)
			if err != nil {
				panic(fmt.Errorf("Git error: %s", err))
			}
		}
	}
}

func managementVersion(c *do.Context) {
	version("management")
}

func version(packageName string) {
	versionFile := filepath.Join(packageName, "version.go")
	os.Remove(versionFile)
	template := `// +build go1.7	
	
package %s

var (
	sdkVersion = "%s"
)
`
	data := []byte(fmt.Sprintf(template, packageName, sdkVersion))
	ioutil.WriteFile(versionFile, data, 0644)
}

func addTasks(fn func(*service), p *do.Project) {
	for _, service := range services {
		s := service
		p.Task(s.TaskName, nil, func(c *do.Context) {
			fn(s)
		})
	}
	p.Task("all", deps, nil)
}

func runner(cmd *exec.Cmd) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if stdout.Len() > 0 {
		fmt.Println(stdout.String())
	}
	if stderr.Len() > 0 {
		fmt.Println(stderr.String())
	}
	return stdout.String(), err
}
