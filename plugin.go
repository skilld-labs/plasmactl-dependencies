// Package plasmactldependencies implements a dependencies launchr plugin
package plasmactldependencies

import (
	"fmt"
	"sort"
	"strings"

	"github.com/launchrctl/launchr"
	"github.com/launchrctl/launchr/pkg/cli"
	plasmactlbump "github.com/skilld-labs/plasmactl-bump"
	"github.com/spf13/cobra"
)

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is launchr plugin providing dependencies search action.
type Plugin struct {
}

// PluginInfo implements launchr.Plugin interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		Weight: 20,
	}
}

// OnAppInit implements launchr.Plugin interface.
func (p *Plugin) OnAppInit(_ launchr.App) error {
	return nil
}

// CobraAddCommands implements launchr.CobraPlugin interface to provide bump functionality.
func (p *Plugin) CobraAddCommands(rootCmd *cobra.Command) error {
	var depCmd = &cobra.Command{
		Use:   "dependencies",
		Short: "Shows parent and child resources of resource",
		Args:  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			showPaths, err := cmd.Flags().GetBool("mrn")
			if err != nil {
				return err
			}

			showTree, err := cmd.Flags().GetBool("tree")
			if err != nil {
				return err
			}

			depth, err := cmd.Flags().GetInt8("depth")
			if err != nil {
				return err
			}
			if depth == 0 {
				return fmt.Errorf("depth value should not be zero")
			}

			return dependencies(args[0], !showPaths, showTree, depth)
		},
	}

	depCmd.Flags().Bool("mrn", false, "Show MRN instead of paths")
	depCmd.Flags().Bool("tree", false, "Show dependencies in tree-like output")
	depCmd.Flags().Int8("depth", 99, "Limit recursion lookup depth")
	depCmd.SetArgs([]string{"target"})

	rootCmd.AddCommand(depCmd)
	return nil
}

func isMachineResourceName(target string) bool {
	list := strings.Split(target, "__")
	return len(list) == 3
}

func convertTarget(target string) (string, error) {
	// @todo take current path as prefix
	r := plasmactlbump.BuildResourceFromPath(target, "")
	if r == nil {
		return "", fmt.Errorf("not valid resource %q", target)
	}

	return r.GetName(), nil
}

func convertToPath(mrn string) string {
	parts := strings.Split(mrn, "__")
	return fmt.Sprintf("%s/%s/roles/%s", parts[0], parts[1], parts[2])
}

func dependencies(target string, toPath, showTree bool, depth int8) error {
	searchMrn := target
	if !isMachineResourceName(searchMrn) {
		converted, err := convertTarget(target)
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

	inv, err := plasmactlbump.NewInventory("empty", ".", "")
	if err != nil {
		return err
	}
	requiredMap := inv.GetRequiredMap()
	parents := lookupDependencies(searchMrn, requiredMap, depth)
	if len(parents) > 0 {
		cli.Println("- Parent dependencies:")
		if showTree {
			var parentsTree forwardTree = requiredMap
			parentsTree.print(header, "", 1, depth, searchMrn, toPath)
		} else {
			printList(parents, toPath)
		}
	}

	dependenciesMap := inv.GetDependenciesMap()
	children := lookupDependencies(searchMrn, dependenciesMap, depth)
	if len(children) > 0 {
		cli.Println("- Child dependencies:")
		if showTree {
			var childrenTree forwardTree = dependenciesMap
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

		cli.Println("%s", res)
	}
}

func lookupDependencies(resourceName string, resourcesMap map[string]map[string]bool, depth int8) map[string]bool {
	result := make(map[string]bool)
	for item := range resourcesMap[resourceName] {
		result[item] = true
		lookupDependenciesRecursively(item, resourcesMap, result, 1, depth)
	}

	return result
}

func lookupDependenciesRecursively(resourceName string, resourcesMap map[string]map[string]bool, result map[string]bool, depth, limit int8) {
	if depth == limit {
		return
	}

	for item := range resourcesMap[resourceName] {
		result[item] = true
		lookupDependenciesRecursively(item, resourcesMap, result, depth+1, limit)
	}
}

type forwardTree map[string]map[string]bool

func (t forwardTree) print(header, indent string, depth, limit int8, parent string, toPath bool) {
	if indent == "" {
		cli.Println(header)
	}

	if depth == limit {
		return
	}

	children, ok := t[parent]
	if !ok {
		return
	}

	keys := make([]string, 0, len(children))
	for k := range children {
		keys = append(keys, k)
	}

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

		cli.Println(indent + edge + value)
		t.print("", newIndent, depth+1, limit, node, toPath)
	}
}
