package aws

import (
	"context"
	htmltemplate "html/template" // want "import 'html/template' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
	"os"                         // want "import 'os' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
	"strings"
	"text/template" // want "import 'text/template' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2v1 "github.com/aws/aws-sdk-go/service/ec2" // want "import 'github.com/aws/aws-sdk-go/service/ec2' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"
)

// Template calls a write by a name in a string, which forbidigo cannot see, so
// the template packages are refused.
func Template(ctx context.Context, client *ec2.Client) error {
	t := template.Must(template.New("x").Parse(`{{.C.TerminateInstances .Ctx nil}}`))
	var b strings.Builder
	if err := t.Execute(&b, map[string]any{"C": client, "Ctx": ctx}); err != nil {
		return err
	}
	return htmltemplate.Must(htmltemplate.New("y").Parse(`{{.C.TerminateInstances .Ctx nil}}`)).Execute(&b, nil)
}

// StartProcess runs the cloud CLI, which no lint rule sees, so os is refused.
func StartProcess() error {
	p, err := os.StartProcess("/usr/bin/aws", []string{"aws", "ec2", "terminate-instances"}, &os.ProcAttr{})
	if err != nil {
		return err
	}
	return p.Release()
}

// V1 uses the AWS SDK v1, which no allowlist covers.
func V1() {
	_ = ec2v1.New(nil) // want "cloud-read-only: use aws-sdk-go-v2 or the Azure SDK"
}
