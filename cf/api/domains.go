package api

import (
	"bytes"
	"encoding/json"

	. "github.com/cloudfoundry/cli/cf/i18n"

	"github.com/cloudfoundry/cli/cf/api/resources"
	"github.com/cloudfoundry/cli/cf/api/strategy"
	"github.com/cloudfoundry/cli/cf/configuration/core_config"
	"github.com/cloudfoundry/cli/cf/errors"
	"github.com/cloudfoundry/cli/cf/models"
	"github.com/cloudfoundry/cli/cf/net"
)

//go:generate counterfeiter -o fakes/fake_domain_repository.go . DomainRepository
type DomainRepository interface {
	ListDomainsForOrg(orgGuid string, cb func(models.DomainFields) bool) error
	FindSharedByName(name string) (models.DomainFields, error)
	FindPrivateByName(name string) (models.DomainFields, error)
	FindByNameInOrg(name string, owningOrgGuid string) (models.DomainFields, error)
	Create(domainName string, owningOrgGuid string) (models.DomainFields, error)
	CreateSharedDomain(domainName string, routerGroupGuid string) error
	Delete(domainGuid string) error
	DeleteSharedDomain(domainGuid string) error
	FirstOrDefault(orgGuid string, name *string) (models.DomainFields, error)
}

type CloudControllerDomainRepository struct {
	config   core_config.Reader
	gateway  net.Gateway
	strategy strategy.EndpointStrategy
}

func NewCloudControllerDomainRepository(config core_config.Reader, gateway net.Gateway, strategy strategy.EndpointStrategy) CloudControllerDomainRepository {
	return CloudControllerDomainRepository{
		config:   config,
		gateway:  gateway,
		strategy: strategy,
	}
}

func (repo CloudControllerDomainRepository) ListDomainsForOrg(orgGuid string, cb func(models.DomainFields) bool) error {
	err := repo.listDomains(repo.strategy.PrivateDomainsByOrgURL(orgGuid), cb)
	if err != nil {
		return err
	}
	err = repo.listDomains(repo.strategy.SharedDomainsURL(), cb)
	return err
}

func (repo CloudControllerDomainRepository) listDomains(path string, cb func(models.DomainFields) bool) error {
	return repo.gateway.ListPaginatedResources(
		repo.config.ApiEndpoint(),
		path,
		resources.DomainResource{},
		func(resource interface{}) bool {
			return cb(resource.(resources.DomainResource).ToFields())
		})
}

func (repo CloudControllerDomainRepository) isOrgDomain(orgGuid string, domain models.DomainFields) bool {
	return orgGuid == domain.OwningOrganizationGuid || domain.Shared
}

func (repo CloudControllerDomainRepository) FindSharedByName(name string) (models.DomainFields, error) {
	return repo.findOneWithPath(repo.strategy.SharedDomainURL(name), name)
}

func (repo CloudControllerDomainRepository) FindPrivateByName(name string) (models.DomainFields, error) {
	return repo.findOneWithPath(repo.strategy.PrivateDomainURL(name), name)
}

func (repo CloudControllerDomainRepository) FindByNameInOrg(name string, orgGuid string) (models.DomainFields, error) {
	domain, err := repo.findOneWithPath(repo.strategy.OrgDomainURL(orgGuid, name), name)

	switch err.(type) {
	case *errors.ModelNotFoundError:
		domain, err = repo.FindSharedByName(name)
		if !domain.Shared {
			err = errors.NewModelNotFoundError("Domain", name)
		}
	}

	if err != nil {
		return models.DomainFields{}, err
	}

	return domain, nil
}

func (repo CloudControllerDomainRepository) findOneWithPath(path, name string) (models.DomainFields, error) {
	foundDomain := false
	var domain models.DomainFields
	err := repo.listDomains(path, func(result models.DomainFields) bool {
		domain = result
		foundDomain = true
		return false
	})

	if err == nil && !foundDomain {
		err = errors.NewModelNotFoundError("Domain", name)
	}

	if err != nil {
		return models.DomainFields{}, err
	}

	return domain, nil
}

func (repo CloudControllerDomainRepository) Create(domainName string, owningOrgGuid string) (models.DomainFields, error) {
	data, err := json.Marshal(resources.DomainEntity{
		Name: domainName,
		OwningOrganizationGuid: owningOrgGuid,
		Wildcard:               true,
	})

	if err != nil {
		return models.DomainFields{}, err
	}

	resource := new(resources.DomainResource)
	err = repo.gateway.CreateResource(
		repo.config.ApiEndpoint(),
		repo.strategy.PrivateDomainsURL(),
		bytes.NewReader(data),
		resource)

	if err != nil {
		return models.DomainFields{}, err
	}

	createdDomain := resource.ToFields()
	return createdDomain, nil
}

func (repo CloudControllerDomainRepository) CreateSharedDomain(domainName string, routerGroupGuid string) error {
	data, err := json.Marshal(resources.DomainEntity{
		Name:            domainName,
		RouterGroupGuid: routerGroupGuid,
		Wildcard:        true,
	})
	if err != nil {
		return err
	}

	return repo.gateway.CreateResource(
		repo.config.ApiEndpoint(),
		repo.strategy.SharedDomainsURL(),
		bytes.NewReader(data),
	)
}

func (repo CloudControllerDomainRepository) Delete(domainGuid string) error {
	return repo.gateway.DeleteResource(
		repo.config.ApiEndpoint(),
		repo.strategy.DeleteDomainURL(domainGuid))
}

func (repo CloudControllerDomainRepository) DeleteSharedDomain(domainGuid string) error {
	return repo.gateway.DeleteResource(
		repo.config.ApiEndpoint(),
		repo.strategy.DeleteSharedDomainURL(domainGuid))
}

func (repo CloudControllerDomainRepository) FirstOrDefault(orgGuid string, name *string) (models.DomainFields, error) {
	var domain models.DomainFields
	var err error
	if name == nil {
		domain, err = repo.defaultDomain(orgGuid)
	} else {
		domain, err = repo.FindByNameInOrg(*name, orgGuid)
	}

	if err != nil {
		return models.DomainFields{}, err
	}

	return domain, nil
}

func (repo CloudControllerDomainRepository) defaultDomain(orgGuid string) (models.DomainFields, error) {
	var foundDomain *models.DomainFields
	repo.ListDomainsForOrg(orgGuid, func(domain models.DomainFields) bool {
		foundDomain = &domain
		return !domain.Shared
	})

	if foundDomain == nil {
		return models.DomainFields{}, errors.New(T("Could not find a default domain"))
	}

	return *foundDomain, nil
}
