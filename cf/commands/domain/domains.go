package domain

import (
	// "fmt"
	"github.com/cloudfoundry/cli/cf/api"
	"github.com/cloudfoundry/cli/cf/command_registry"
	"github.com/cloudfoundry/cli/cf/configuration/core_config"
	. "github.com/cloudfoundry/cli/cf/i18n"
	"github.com/cloudfoundry/cli/cf/models"
	"github.com/cloudfoundry/cli/cf/requirements"
	"github.com/cloudfoundry/cli/cf/terminal"
	"github.com/cloudfoundry/cli/flags"
)

type ListDomains struct {
	ui                     terminal.UI
	config                 core_config.Reader
	domainRepo             api.DomainRepository
	routingApiRepo         api.RoutingApiRepository
	rountingApiRequirement requirements.Requirement
}

func init() {
	command_registry.Register(&ListDomains{})
}

func (cmd *ListDomains) MetaData() command_registry.CommandMetadata {
	return command_registry.CommandMetadata{
		Name:        "domains",
		Description: T("List domains in the target org"),
		Usage:       "CF_NAME domains",
	}
}

func (cmd *ListDomains) Requirements(requirementsFactory requirements.Factory, fc flags.FlagContext) ([]requirements.Requirement, error) {
	if len(fc.Args()) != 0 {
		cmd.ui.Failed(T("Incorrect Usage. No argument required\n\n") + command_registry.Commands.CommandUsage("domains"))
	}

	cmd.rountingApiRequirement = requirementsFactory.NewRoutingAPIRequirement()

	reqs := []requirements.Requirement{
		requirementsFactory.NewLoginRequirement(),
		requirementsFactory.NewTargetedOrgRequirement(),
	}

	return reqs, nil
}

func (cmd *ListDomains) SetDependency(deps command_registry.Dependency, pluginCall bool) command_registry.Command {
	cmd.ui = deps.Ui
	cmd.config = deps.Config
	cmd.domainRepo = deps.RepoLocator.GetDomainRepository()
	cmd.routingApiRepo = deps.RepoLocator.GetRoutingApiRepository()

	return cmd
}

func (cmd *ListDomains) Execute(c flags.FlagContext) {
	org := cmd.config.OrganizationFields()

	cmd.ui.Say(T("Getting domains in org {{.OrgName}} as {{.Username}}...",
		map[string]interface{}{
			"OrgName":  terminal.EntityNameColor(org.Name),
			"Username": terminal.EntityNameColor(cmd.config.Username())}))

	domains, populateRouterGroups, err := cmd.getDomains(org.Guid)
	if err != nil {
		cmd.ui.Failed(T("Failed fetching domains.\n{{.Error}}", map[string]interface{}{"Error": err.Error()}))
	}

	if len(domains) == 0 {
		cmd.ui.Say(T("No domains found"))
		return
	}

	var routerGroups map[string]models.RouterGroup
	if populateRouterGroups {
		cmd.rountingApiRequirement.Execute()

		routerGroups, err = cmd.getRouterGroups()
		if err != nil {
			cmd.ui.Failed(T("Failed fetching router groups.\n{{.Err}}", map[string]interface{}{"Err": err.Error()}))
			return
		}

		// validate that all shared domains with router group id have valid router group id
		for _, domain := range domains {
			if domain.Shared && domain.RouterGroupGuid != "" {
				if _, ok := routerGroups[domain.RouterGroupGuid]; !ok {
					cmd.ui.Failed(T("Invalid router group guid: {{.Guid}}", map[string]interface{}{"Guid": domain.RouterGroupGuid}))
				}
			}
		}
	}

	cmd.printDomainsTable(domains, routerGroups)
}

func (cmd *ListDomains) getDomains(orgGuid string) ([]models.DomainFields, bool, error) {
	domains := []models.DomainFields{}
	populateRouterGroups := false
	err := cmd.domainRepo.ListDomainsForOrg(orgGuid, func(domain models.DomainFields) bool {
		domains = append(domains, domain)
		if domain.Shared && domain.RouterGroupGuid != "" {
			populateRouterGroups = true
		}
		return true
	})

	if err != nil {
		return []models.DomainFields{}, false, err
	}

	return domains, populateRouterGroups, nil
}

func (cmd *ListDomains) printDomainsTable(domains []models.DomainFields, routerGroups map[string]models.RouterGroup) {

	table := cmd.ui.Table([]string{T("name"), T("status"), T("type")})

	for _, domain := range domains {
		if domain.Shared {
			if domain.RouterGroupGuid != "" {
				table.Add(domain.Name, T("shared"), routerGroups[domain.RouterGroupGuid].Type)
			} else {
				table.Add(domain.Name, T("shared"))
			}
		}
	}

	// separate loop to keep the private domains at the end as the domains
	// are printed in the order they are added
	for _, domain := range domains {
		if !domain.Shared {
			table.Add(domain.Name, T("owned"))
		}
	}

	table.Print()
}

func (cmd *ListDomains) getRouterGroups() (map[string]models.RouterGroup, error) {
	routerGroupsMap := map[string]models.RouterGroup{}
	cb := func(routerGroup models.RouterGroup) bool {
		routerGroupsMap[routerGroup.Guid] = routerGroup
		return true
	}

	err := cmd.routingApiRepo.ListRouterGroups(cb)
	if err != nil {
		return map[string]models.RouterGroup{}, err
	}

	return routerGroupsMap, nil
}
