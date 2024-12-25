// Package plasmactldependencies implements a dependencies launchr plugin
package plasmactldependencies

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/action"

	"github.com/skilld-labs/plasmactl-bump/v2/pkg/sync"
)

//go:embed action.yaml
var actionYaml []byte

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is [launchr.Plugin] providing dependencies search action.
type Plugin struct {
}

// PluginInfo implements [launchr.Plugin] interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		Weight: 20,
	}
}

// DiscoverActions implements [launchr.ActionDiscoveryPlugin] interface.
func (p *Plugin) DiscoverActions(_ context.Context) ([]*action.Action, error) {
	a := action.NewFromYAML("dependencies", actionYaml)
	a.SetRuntime(action.NewFnRuntime(func(_ context.Context, a *action.Action) error {
		input := a.Input()
		source := input.Opt("source").(string)
		if _, err := os.Stat(source); os.IsNotExist(err) {
			launchr.Term().Warning().Printfln("%s doesn't exist, fallback to current dir", source)
			source = "."
		} else {
			launchr.Term().Info().Printfln("Selected source is %s", source)
		}

		showPaths := input.Opt("mrn").(bool)
		showTree := input.Opt("tree").(bool)
		depth := int8(input.Opt("depth").(int)) //nolint:gosec
		if depth == 0 {
			return fmt.Errorf("depth value should not be zero")
		}

		target := input.Arg("target").(string)
		return dependencies(target, source, !showPaths, showTree, depth)
	}))
	return []*action.Action{a}, nil
}

func isMachineResourceName(target string) bool {
	list := strings.Split(target, "__")
	return len(list) == 3
}

func convertTarget(source, target string) (string, error) {
	r := sync.BuildResourceFromPath(target, source)
	if r == nil {
		return "", fmt.Errorf("not valid resource %q", target)
	}

	return r.GetName(), nil
}

func convertToPath(mrn string) string {
	parts := strings.Split(mrn, "__")
	return filepath.Join(parts[0], parts[1], "roles", parts[2])
}

func dependencies(target, source string, toPath, showTree bool, depth int8) error {
	searchMrn := target
	if !isMachineResourceName(searchMrn) {
		converted, err := convertTarget(source, target)
		if err != nil {
			return err
		}

		searchMrn = converted
	}

	var header string
	if toPath {
		header = convertToPath(searchMrn)
	} else {
		header = searchMrn
	}

	// @TODO move inventory into dependencies?
	inv, err := sync.NewInventory(source)
	if err != nil {
		return err
	}
	parents := inv.GetRequiredByResources(searchMrn, depth)
	if len(parents) > 0 {
		launchr.Term().Info().Println("Dependent resources:")
		if showTree {
			var parentsTree forwardTree = inv.GetRequiredByMap()
			parentsTree.print(header, "", 1, depth, searchMrn, toPath)
		} else {
			printList(parents, toPath)
		}
	}

	children := inv.GetDependsOnResources(searchMrn, depth)
	if len(children) > 0 {
		launchr.Term().Info().Println("Dependencies:")
		if showTree {
			var childrenTree forwardTree = inv.GetDependsOnMap()
			childrenTree.print(header, "", 1, depth, searchMrn, toPath)
		} else {
			printList(children, toPath)
		}
	}

	return nil
}

func printList(items map[string]bool, toPath bool) {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, item := range keys {
		res := item
		if toPath {
			res = convertToPath(res)
		}

		launchr.Term().Print(res + "\n")
	}
}

type forwardTree map[string]*sync.OrderedMap[bool]

func (t forwardTree) print(header, indent string, depth, limit int8, parent string, toPath bool) {
	if indent == "" {
		launchr.Term().Printfln(header)
	}

	if depth == limit {
		return
	}

	children, ok := t[parent]
	if !ok {
		return
	}

	keys := children.Keys()
	sort.Strings(keys)

	for i, node := range keys {
		isLast := i == len(keys)-1
		var newIndent, edge string

		if isLast {
			newIndent = indent + "    "
			edge = "└── "
		} else {
			newIndent = indent + "│   "
			edge = "├── "
		}
		value := node
		if toPath {
			value = convertToPath(value)
		}

		launchr.Term().Printfln(indent + edge + value)
		t.print("", newIndent, depth+1, limit, node, toPath)
	}
}
