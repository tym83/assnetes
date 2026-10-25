package cli

import (
	"fmt"
	"strings"

	kubepkgcli "github.com/kuberoot-dev/kubepkg/pkg/cli"
	"github.com/spf13/cobra"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"
)

// GroupPakety is the API group of packages, «пакеты.бух».
const GroupPakety = "xn--80akoss8c.xn--90a0bg"

// Russian names of the kubepkg commands, the way an accountant buys things.
var paketyNames = map[string][]string{
	"install":  {"закупить"},
	"remove":   {"списать"},
	"list":     {"остатки"},
	"search":   {"прайс"},
	"plan":     {"смета"},
	"history":  {"история"},
	"rollback": {"сторно"},
	"repo":     {"поставщики"},
	"adopt":    {"оприходовать"},
}

// newPaketyCommand embeds kubepkg as «1сctl пакеты» under the API group of
// the distribution, with Russian names, and the everyday ones on top:
// «1сctl закупить бухгалтерия».
func newPaketyCommand(root *cobra.Command) {
	opts := kubepkgcli.DefaultOptions()
	opts.Name = "пакеты"
	opts.Short = "пакетный менеджер: закупить, списать, остатки, прайс, смета, сторно"
	opts.APIGroup = GroupPakety
	pk := kubepkgcli.NewRootCommand(opts)
	// On a cluster whose package manager serves kubepkg's own group, as
	// the kuberoot distribution does, take that group instead.
	pk.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if f := cmd.Flags().Lookup("api-group"); f != nil && !f.Changed {
			if g := servedPackageGroup(); g != "" {
				_ = f.Value.Set(g)
			}
		}
		return nil
	}
	pk.Aliases = []string{"packages"}
	for _, c := range pk.Commands() {
		if ru, ok := paketyNames[c.Name()]; ok {
			c.Aliases = append(c.Aliases, ru...)
		}
	}
	root.AddCommand(pk)
	for _, en := range []string{"install", "remove", "list", "search", "rollback"} {
		ru := paketyNames[en][0]
		target := en
		root.AddCommand(&cobra.Command{
			Use:                ru,
			Short:              fmt.Sprintf("то же, что «1сctl пакеты %s»", ru),
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				// cobra executes a subcommand through its root: run the root
				// again, as «1сctl пакеты <command>»
				root.SetArgs(append([]string{"пакеты", target}, translatePackages(args)...))
				return root.ExecuteContext(cmd.Context())
			},
		})
	}
}

// packageNames are the products of the price list by the names people
// call them.
var packageNames = map[string]string{
	"битрикс": "1c-bitrix", "1с-битрикс": "1c-bitrix", "сайт": "1c-bitrix",
	"обработки": "assnetes-obrabotki", "всё": "assnetes", "все": "assnetes",
	"предприятие": "1c-predpriyatie", "платформа": "1c-predpriyatie",
	"бухгалтерия": "1c-buhgalteriya", "торговля": "1c-upravlenie-torgovlej",
	"управлениеторговлей": "1c-upravlenie-torgovlej", "црм": "1c-crm", "crm": "1c-crm",
}

func translatePackages(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		name, constraint, hasAt := strings.Cut(a, "@")
		if p, ok := packageNames[strings.ToLower(name)]; ok && !strings.HasPrefix(a, "-") {
			name = p
		}
		out[i] = name
		if hasAt {
			out[i] += "@" + constraint
		}
	}
	return out
}

// servedPackageGroup finds which group the cluster's package manager
// serves: «пакеты.бух» first, then kubepkg.dev.
func servedPackageGroup() string {
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return ""
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return ""
	}
	groups, err := dc.ServerGroups()
	if err != nil {
		return ""
	}
	found := map[string]bool{}
	for _, g := range groups.Groups {
		found[g.Name] = true
	}
	for _, g := range []string{GroupPakety, "kubepkg.dev"} {
		if found[g] {
			return g
		}
	}
	return ""
}
