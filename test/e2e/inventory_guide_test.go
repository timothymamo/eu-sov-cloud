//go:build e2e

package e2e_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/timothymamo/shift2eu/internal/testutil"
)

// This file pins the jq programs docs/guide/inventory.md documents (issue #178):
// each program turns the recorded output of the guide's read-only CLI calls into
// a schema version 1 inventory the binary accepts. The programs are part of what
// the guide promises, so they are run as the guide runs them, against the
// recorded fixtures, not just read.

const (
	// inventoryGuideDir holds the recorded, sanitised CLI output of the guide,
	// one directory per source.
	inventoryGuideDir = "testdata/inventory-guide"
	// jqProgramDir holds the programs the guide runs.
	jqProgramDir = "../../docs/guide/inventory"
	// inventoryGuideGoldenDir holds the inventories the programs write. It is a
	// directory of its own because the gcp and azure programs read every
	// <source>/*.json file: a golden beside them would be an input too.
	inventoryGuideGoldenDir = "inventory-guide/golden"
	// schemaVersion is the only schema version the binary reads (ADR 0006).
	schemaVersion = 1
)

// inventoryGuideCase is one source the guide covers: the recorded CLI output its
// program reads, and the inventory the program writes for it.
type inventoryGuideCase struct {
	// source is the inventory's source and the program's name without .jq.
	source string
	// pattern is the glob, relative to this package, of the recorded CLI output.
	// The AWS calls are per region, so its pattern has one more level.
	pattern string
	// golden is the inventory the program writes for the recorded output. It is
	// named .golden rather than .json because .gitignore excludes
	// *.inventory.json: a real inventory describes someone's infrastructure,
	// while this one is a sanitised fixture and has to be committed.
	golden string
	// bareMetal are the recorded bare-metal servers of this source: the
	// resources the program has to classify compute.baremetal, never compute.vm
	// (ADR 0023). One source spells it in more than one way, so a source may
	// record several.
	bareMetal []bareMetalServer
	// present are resources the inventory has to hold, by ID, with this category
	// and rawType.
	present []resourceFact
	// absent are resource IDs the program has to leave out altogether: another
	// recorded resource stands for them.
	absent []string
	// absentRawTypes are raw types no resource of this source may carry.
	absentRawTypes []string
}

// bareMetalServer is one bare-metal server a source's recorded output holds.
// Every provider spells it differently, so the case names the ID and the raw
// type the guide's program writes for it.
type bareMetalServer struct {
	// id is the short ID the program writes for the server.
	id string
	// rawType is the source's own type for it.
	rawType string
}

// resourceFact is what the inventory has to say about one resource, by ID. The
// assertions on a resourceFact are separate from the golden file on purpose: a
// careless -update rewrites the golden alone, so it must not be able to accept
// a mapping away.
type resourceFact struct {
	id       string
	rawType  string
	category string
}

// inventoryGuides returns the sources the guide covers. A source added to the
// guide gets a row here, its fixtures and a golden file.
func inventoryGuides() []inventoryGuideCase {
	return []inventoryGuideCase{
		{
			source:  "aws",
			pattern: inventoryGuideDir + "/aws/*/*.json",
			golden:  "aws.inventory.golden",
			bareMetal: []bareMetalServer{
				// An instance type ending in .metal is a dedicated physical
				// server, and so is a metal type with a size suffix.
				{id: "i-0123456789abcdef6", rawType: "AWS::EC2::Instance"},
				{id: "i-0123456789abcdef7", rawType: "AWS::EC2::Instance"},
			},
			present: []resourceFact{
				// An Aurora cluster is covered by its instances, which
				// rds describe-db-instances lists.
				{id: "shop-aurora-1", rawType: "AWS::RDS::DBInstance", category: "database.postgresql"},
				// A volume that outlives its node is a persistent volume and
				// stays: only the node's own root volume is left out.
				{id: "vol-0123456789abcdef6", rawType: "AWS::EC2::Volume", category: "storage.block"},
			},
			// The EKS nodes, the Auto Scaling groups that run them and their
			// root volumes, and the root volume of an Auto Scaling group's
			// instance: the group stands for its instances.
			absent: []string{
				"i-0123456789abcdef4",
				"i-0123456789abcdef5",
				"vol-0123456789abcdef2",
				"vol-0123456789abcdef4",
				"vol-0123456789abcdef5",
				"eks-shop-eks-default-00000000-0000-0000-0000-000000000000",
				"shop-eks-self-managed",
			},
			absentRawTypes: []string{"AWS::RDS::DBCluster"},
		},
		{
			source:    "gcp",
			pattern:   inventoryGuideDir + "/gcp/*.json",
			golden:    "gcp.inventory.golden",
			bareMetal: []bareMetalServer{{id: "shop-baremetal-1", rawType: "baremetalsolution.googleapis.com/Instance"}},
			present: []resourceFact{
				// A persistent volume is a disk no Kubernetes node label covers:
				// it holds data and stays, only a node's boot disk goes.
				{id: "pvc-00000000-0000-0000-0000-000000000001", rawType: "compute.googleapis.com/Disk", category: "storage.block"},
			},
			// A node pool's managed instance group, a node with its disk,
			// labelled goog-gke-node, and the boot disk of the group's member:
			// the group stands for its instances.
			absent: []string{
				"gke-shop-gke-default-pool-0a1b2c3d-grp",
				"gke-shop-gke-default-pool-0a1b2c3d-x1y2",
				"shop-web-mig-0abc",
			},
		},
		{
			source:  "azure",
			pattern: inventoryGuideDir + "/azure/*.json",
			golden:  "azure.inventory.golden",
			bareMetal: []bareMetalServer{
				{id: "shop-baremetal-1", rawType: "Microsoft.BareMetalInfrastructure/bareMetalInstances"},
				// A SAP HANA Large Instance is a dedicated physical server too.
				{id: "shop-hana-1", rawType: "Microsoft.AzureLargeInstance/azureLargeInstances"},
			},
			present: []resourceFact{
				// A storage queue is a queue; a Service Bus topic publishes and
				// subscribes, which no taxonomy category covers.
				{id: "shop-thumbnail-jobs", rawType: "Microsoft.Storage/storageAccounts/queueServices/queues", category: "messaging.queue"},
				{id: "sb-shop-order-events", rawType: "Microsoft.ServiceBus/namespaces/topics", category: "unclassified"},
				// A persistent volume in the node resource group holds data and
				// stays: only what AKS manages for the cluster is left out.
				{id: "pvc-00000000-0000-0000-0000-000000000000", rawType: "Microsoft.Compute/disks", category: "storage.block"},
			},
			// The cluster's node scale set and its load balancer.
			absent: []string{
				"aks-system-12345678-vmss",
				"kubernetes",
			},
		},
	}
}

// taxonomyCategories are the taxonomy categories the recorded fixtures cover.
// They are written out because this package is black box and imports no domain
// package (the e2e-is-black-box depguard rule), and because a -update that
// dropped a mapping must not be able to hide itself: the inventory is checked
// against this list, not against the golden file it was written from.
func taxonomyCategories() []string {
	return []string{
		"compute.baremetal",
		"compute.vm",
		"storage.block",
		"storage.object",
		"database.postgresql",
		"database.mysql",
		"kubernetes.managed",
		"serverless.function",
		"messaging.queue",
	}
}

// inventoryDoc is the part of a schema version 1 inventory this file checks.
type inventoryDoc struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Source        string              `json:"source"`
	Resources     []inventoryResource `json:"resources"`
}

// inventoryResource is the part of one resource this file checks.
type inventoryResource struct {
	ID       string `json:"id"`
	RawType  string `json:"rawType"`
	Category string `json:"category"`
	Region   string `json:"region"`
}

// findResource returns the resource with id, and whether the inventory holds it.
func findResource(doc inventoryDoc, id string) (inventoryResource, bool) {
	for _, resource := range doc.Resources {
		if resource.ID == id {
			return resource, true
		}
	}

	return inventoryResource{}, false
}

// jqPath returns the path of the jq every release target has installed. A test
// that cannot run the programs fails instead of skipping: the guide's promise
// would otherwise go unchecked wherever jq is missing.
func jqPath(t *testing.T) string {
	t.Helper()

	path, err := exec.LookPath("jq")
	if err != nil {
		t.Fatalf("jq is not on PATH; the inventory guide's programs need it (install jq): %v", err)
	}

	return path
}

// TestInventoryGuidePrograms pins docs/guide/inventory/<source>.jq end to end:
// the program, run as the guide runs it on the recorded CLI output, writes the
// golden inventory, and the built binary assesses that inventory against every
// target without an error. The assertions beyond the golden file are the
// guide's promises that a careless -update must not be able to accept away: the
// document's schema version and source, every taxonomy category the fixtures
// cover, the bare-metal servers of each source (ADR 0023), the resources the
// program has to leave out or classify a particular way, an unclassified
// resource that keeps its rawType, and short IDs only.
func TestInventoryGuidePrograms(t *testing.T) {
	t.Parallel()

	jq := jqPath(t)

	for _, tc := range inventoryGuides() {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()

			inputs, err := filepath.Glob(tc.pattern)
			if err != nil {
				t.Fatalf("glob %s: %v", tc.pattern, err)
			}

			if len(inputs) == 0 {
				t.Fatalf("%s matched no recorded CLI output; the fixtures are missing", tc.pattern)
			}

			slices.Sort(inputs)

			stdout := runJQ(t, jq, tc.source, inputs)

			testutil.Golden(t, filepath.Join("testdata", inventoryGuideGoldenDir, tc.golden), stdout)

			doc := decodeInventory(t, stdout)

			if doc.SchemaVersion != schemaVersion {
				t.Errorf("schemaVersion = %d, want %d", doc.SchemaVersion, schemaVersion)
			}

			if doc.Source != tc.source {
				t.Errorf("source = %q, want %q", doc.Source, tc.source)
			}

			if len(doc.Resources) == 0 {
				t.Fatalf("the inventory holds no resource; the program classified nothing\n%s", stdout)
			}

			assertCategories(t, tc.source, doc)
			assertBareMetal(t, tc.source, doc, tc.bareMetal)
			assertPresent(t, tc.source, doc, tc.present)
			assertAbsent(t, tc.source, doc, tc.absent)
			assertNoRawTypes(t, tc.source, doc, tc.absentRawTypes)
			assertUnclassifiedKeepsRawType(t, tc.source, doc)
			assertShortIDs(t, tc.source, doc)

			assertAssessAccepts(t, tc, stdout)
		})
	}
}

// runJQ runs jq -n -f the source's program on the recorded CLI output, as the
// guide does, and returns what it wrote to stdout. The program must exit 0 with
// nothing on stderr, so a warning a reader would not notice fails the test.
func runJQ(t *testing.T, jq, source string, inputs []string) []byte {
	t.Helper()

	args := append([]string{"-n", "-f", filepath.Join(jqProgramDir, source+".jq")}, inputs...)

	cmd := exec.Command(jq, args...)

	var stdout, stderr strings.Builder

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// The program needs no environment: it reads the files the arguments name.
	cmd.Env = emptyEnv()

	err := cmd.Run()

	var exitErr *exec.ExitError

	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		t.Fatalf("jq -n -f %s %v exited %d, want 0; stderr = %q",
			source+".jq", inputs, exitErr.ExitCode(), stderr.String())
	default:
		t.Fatalf("running jq on %s's recorded CLI output: %v", source, err)
	}

	if stderr.Len() != 0 {
		t.Errorf("jq wrote to stderr, want nothing: %q", stderr.String())
	}

	return []byte(stdout.String())
}

// decodeInventory decodes the document the program wrote. The binary has to
// decode it too, so a document that does not decode fails here first.
func decodeInventory(t *testing.T, document []byte) inventoryDoc {
	t.Helper()

	var doc inventoryDoc
	if err := json.Unmarshal(document, &doc); err != nil {
		t.Fatalf("the program's output is not a JSON inventory: %v\n%s", err, document)
	}

	return doc
}

// assertCategories fails unless every taxonomy category the fixtures cover
// appears in the inventory. A category that no longer appears is a mapping the
// program lost, or a fixture it no longer reads.
func assertCategories(t *testing.T, source string, doc inventoryDoc) {
	t.Helper()

	got := make([]string, 0, len(doc.Resources))
	for _, resource := range doc.Resources {
		got = append(got, resource.Category)
	}

	slices.Sort(got)
	got = slices.Compact(got)

	want := append(taxonomyCategories(), "unclassified")
	slices.Sort(want)
	want = slices.Compact(want)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("%s: the inventory's categories do not cover the fixture (-want +got):\n%s", source, diff)
	}
}

// assertBareMetal fails unless the inventory holds each recorded bare-metal
// server, by ID, as compute.baremetal with the source's raw type. A bare-metal
// server is not a virtual machine (ADR 0023): moving to it changes how the
// resource is provisioned, sized, snapshotted and billed, so classifying it
// compute.vm assesses it against hosts that cannot replace it.
func assertBareMetal(t *testing.T, source string, doc inventoryDoc, wants []bareMetalServer) {
	t.Helper()

	for _, want := range wants {
		resource, ok := findResource(doc, want.id)
		if !ok {
			t.Errorf("%s: the inventory holds no resource %q; the recorded bare-metal server is missing",
				source, want.id)

			continue
		}

		if resource.Category != "compute.baremetal" {
			t.Errorf("%s: bare-metal server %q has category %q, want %q",
				source, want.id, resource.Category, "compute.baremetal")
		}

		if resource.RawType != want.rawType {
			t.Errorf("%s: bare-metal server %q has rawType %q, want %q",
				source, want.id, resource.RawType, want.rawType)
		}
	}
}

// assertPresent fails unless the inventory holds each fact's resource, by ID,
// with its category and raw type.
func assertPresent(t *testing.T, source string, doc inventoryDoc, wants []resourceFact) {
	t.Helper()

	for _, want := range wants {
		resource, ok := findResource(doc, want.id)
		if !ok {
			t.Errorf("%s: the inventory holds no resource %q", source, want.id)

			continue
		}

		if resource.Category != want.category {
			t.Errorf("%s: resource %q has category %q, want %q",
				source, want.id, resource.Category, want.category)
		}

		if resource.RawType != want.rawType {
			t.Errorf("%s: resource %q has rawType %q, want %q",
				source, want.id, resource.RawType, want.rawType)
		}
	}
}

// assertAbsent fails when the inventory holds any of the IDs. The nodes of a
// managed Kubernetes cluster are left out, the cluster stands for them, and so
// is what the cluster or a group manages for them: an inventory that lists them
// assesses every node as a machine of its own.
func assertAbsent(t *testing.T, source string, doc inventoryDoc, ids []string) {
	t.Helper()

	for _, id := range ids {
		resource, ok := findResource(doc, id)
		if !ok {
			continue
		}

		t.Errorf("%s: the inventory holds %q (%s, %s), want it left out: another recorded resource stands for it",
			source, id, resource.RawType, resource.Category)
	}
}

// assertNoRawTypes fails when a resource carries one of the raw types. The
// entries name a resource that another, listed resource covers: a cluster is
// assessed per instance, so listing it too would count it twice.
func assertNoRawTypes(t *testing.T, source string, doc inventoryDoc, rawTypes []string) {
	t.Helper()

	for _, resource := range doc.Resources {
		if slices.Contains(rawTypes, resource.RawType) {
			t.Errorf("%s: resource %q has rawType %q, want none: another resource covers it",
				source, resource.ID, resource.RawType)
		}
	}
}

// assertUnclassifiedKeepsRawType fails unless the inventory holds an
// unclassified resource with a rawType. An unclassified resource without one
// tells a reader what blocks the move even less than the category already does.
func assertUnclassifiedKeepsRawType(t *testing.T, source string, doc inventoryDoc) {
	t.Helper()

	var unclassified []string

	for _, resource := range doc.Resources {
		if resource.Category != "unclassified" {
			continue
		}

		if resource.RawType == "" {
			t.Errorf("%s: unclassified resource %q has no rawType", source, resource.ID)

			continue
		}

		unclassified = append(unclassified, resource.ID)
	}

	if len(unclassified) == 0 {
		t.Errorf("%s: the inventory holds no unclassified resource with a rawType, want at least one", source)
	}
}

// assertShortIDs fails when a resource ID still carries an ARN, an Azure
// resource ID or a GCP resource name: those hold the account, subscription or
// project, which must not reach an inventory or a report.
func assertShortIDs(t *testing.T, source string, doc inventoryDoc) {
	t.Helper()

	long := []string{"arn:", "/subscriptions/", "projects/"}

	for _, resource := range doc.Resources {
		for _, marker := range long {
			if strings.Contains(resource.ID, marker) {
				t.Errorf("%s: resource id %q holds %q; the guide's programs write short ids only",
					source, resource.ID, marker)
			}
		}
	}
}

// assertAssessAccepts runs the built binary on the inventory the program wrote,
// against every target, and fails unless it exits 0 with nothing on stderr. The
// output goes to a temporary directory, so no fixture is written by a test.
func assertAssessAccepts(t *testing.T, tc inventoryGuideCase, document []byte) {
	t.Helper()

	inventory := filepath.Join(t.TempDir(), tc.golden)
	if err := os.WriteFile(inventory, document, 0o600); err != nil {
		t.Fatalf("write %s: %v", inventory, err)
	}

	got := assess(t, allKey, inventory)

	if got.code != 0 {
		t.Fatalf("shift2eu assess --target %s on the %s inventory exited %d, want 0; stderr = %q",
			allKey, tc.source, got.code, got.stderr)
	}

	if got.stderr != "" {
		t.Errorf("assess wrote to stderr, want nothing: %q", got.stderr)
	}

	// The report names the source the program wrote, so a document that
	// decodes but says nothing usable still fails here.
	want := "- Source: `" + tc.source + "`"

	if !strings.Contains(got.stdout, want) {
		t.Errorf("the report does not hold %q:\n%s", want, got.stdout)
	}
}
