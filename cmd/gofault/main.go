// Package cmd provides CLI commands for gofault.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// version is the CLI version. Keep it in step with the tagged release; the
// framework itself is versioned in gofault/CHANGELOG.md.
const version = "v3.4.1"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "new":
		if err := runNew(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("gofault " + version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`gofault - A lightweight Go web framework

Usage:
    gofault <command> [arguments]

Commands:
    new [name]    Create a new gofault project
    version       Print the version number
    help          Show this help message

Examples:
    gofault new myapp
    gofault version`)
}

func runNew() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("project name required\nUsage: gofault new <project-name>")
	}
	name := os.Args[2]
	return createProject(name)
}

// validateProjectName rejects names that are not a single safe path segment.
// The name is used both as a directory under the working directory and as the
// Go module path, so "../evil" or an absolute path would either write outside
// the current directory or produce a module path that cannot resolve.
func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid project name %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("project name %q must not contain a path separator", name)
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("project name contains a NUL byte")
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("project name contains a control character")
		}
	}
	return nil
}

func createProject(name string) error {
	if err := validateProjectName(name); err != nil {
		return err
	}

	// Determine working directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Confirm the joined path really lands inside wd before creating anything.
	projectDir := filepath.Join(wd, name)
	absWD, err := filepath.Abs(wd)
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}
	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		return fmt.Errorf("failed to resolve project directory: %w", err)
	}
	if absProject == absWD || !strings.HasPrefix(absProject, absWD+string(os.PathSeparator)) {
		return fmt.Errorf("project directory %q escapes the working directory", name)
	}

	// Create project directory
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// Generate content from templates
	mainContent, err := executeTemplate(mainGoTemplate, map[string]string{"Name": name})
	if err != nil {
		return fmt.Errorf("failed to generate main.go: %w", err)
	}

	goModContent, err := executeTemplate(goModTemplate, map[string]string{"Name": name})
	if err != nil {
		return fmt.Errorf("failed to generate go.mod: %w", err)
	}

	configContent, err := executeTemplate(configYamlTemplate, map[string]string{"Name": name})
	if err != nil {
		return fmt.Errorf("failed to generate config.yaml: %w", err)
	}

	ctrlContent, err := executeTemplate(controllerTemplate, map[string]string{"Name": name})
	if err != nil {
		return fmt.Errorf("failed to generate controller: %w", err)
	}

	files := map[string]string{
		"main.go":              mainContent,
		"go.mod":               goModContent,
		"config.yaml":          configContent,
		"controllers/hello.go": ctrlContent,
	}

	for path, content := range files {
		fullPath := filepath.Join(projectDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	fmt.Printf("✓ Created project %q\n", name)
	fmt.Println("\nProject structure:")
	fmt.Printf("  %s/\n", name)
	fmt.Printf("  ├── main.go\n")
	fmt.Printf("  ├── go.mod\n")
	fmt.Printf("  ├── config.yaml\n")
	fmt.Printf("  └── controllers/\n")
	fmt.Printf("      └── hello.go\n")
	fmt.Println("\nTo get started:")
	fmt.Printf("  cd %s && go mod tidy && go run main.go\n", name)
	return nil
}

func executeTemplate(tmpl string, data map[string]string) (string, error) {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var result strings.Builder
	if err := t.Execute(&result, data); err != nil {
		return "", err
	}
	return result.String(), nil
}

const mainGoTemplate = `package main

import (
	"fmt"
	"log"

	"github.com/gofault/gofault/core"
	"github.com/gofault/gofault/module"
	"{{.Name}}/controllers"
)

func main() {
	// Create application
	app := module.New()

	// Create module
	mod := core.NewModule("{{.Name}}")
	mod.RegisterControllers(controllers.NewHelloController())

	// Register module
	if err := app.RegisterModules(mod); err != nil {
		log.Fatalf("Failed to register module: %v", err)
	}

	// Start server
	fmt.Println("Server starting on :8080...")
	if err := app.Start(8080); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
`

const goModTemplate = `module {{.Name}}

go 1.25

require github.com/gofault/gofault v0.0.0

replace github.com/gofault/gofault => ../gofault
`

const configYamlTemplate = `# Application Configuration
app:
  name: {{.Name}}
  port: 8080
  env: development

logging:
  level: info
  format: json
`

const controllerTemplate = `package controllers

import (
	"github.com/gofault/gofault/core"
)

// HelloController handles hello-related routes.
type HelloController struct{}

func NewHelloController() *HelloController {
	return &HelloController{}
}

// Routes returns the routes for this controller.
func (c *HelloController) Routes() []core.Route {
	return []core.Route{
		{Method: "GET", Path: "/", Handler: "Index"},
		{Method: "GET", Path: "/hello/:name", Handler: "Greet"},
	}
}

// Prefix returns the route prefix for this controller.
func (c *HelloController) Prefix() string {
	return "/api/v1"
}

// Index handles GET /
func (c *HelloController) Index(ctx *core.Ctx) error {
	_, err := ctx.Response.Write([]byte("Hello, GoFault!"))
	return err
}

// Greet handles GET /hello/:name
func (c *HelloController) Greet(ctx *core.Ctx) error {
	name := ctx.Params["name"]
	_, err := ctx.Response.Write([]byte("Hello, " + name + "!"))
	return err
}
`
