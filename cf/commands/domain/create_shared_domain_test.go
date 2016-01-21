package domain_test

import (
	"errors"

	"github.com/blang/semver"
	"github.com/cloudfoundry/cli/cf/command_registry"
	"github.com/cloudfoundry/cli/cf/configuration/core_config"
	"github.com/cloudfoundry/cli/cf/requirements"
	"github.com/cloudfoundry/cli/flags"

	fakeapi "github.com/cloudfoundry/cli/cf/api/fakes"
	fakerequirements "github.com/cloudfoundry/cli/cf/requirements/fakes"
	testconfig "github.com/cloudfoundry/cli/testhelpers/configuration"
	testterm "github.com/cloudfoundry/cli/testhelpers/terminal"

	. "github.com/cloudfoundry/cli/testhelpers/matchers"

	"github.com/cloudfoundry/cli/cf/commands/domain"
	"github.com/cloudfoundry/cli/cf/models"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type passingRequirement struct {
	Name string
}

func (r passingRequirement) Execute() bool {
	return true
}

var _ = Describe("CreateSharedDomain", func() {
	var (
		ui             *testterm.FakeUI
		routingApiRepo *fakeapi.FakeRoutingApiRepository
		domainRepo     *fakeapi.FakeDomainRepository
		configRepo     core_config.Repository

		cmd         domain.CreateSharedDomain
		deps        command_registry.Dependency
		factory     *fakerequirements.FakeFactory
		flagContext flags.FlagContext

		loginRequirement         requirements.Requirement
		routingApiRequirement    requirements.Requirement
		minAPIVersionRequirement requirements.Requirement
	)

	BeforeEach(func() {
		ui = &testterm.FakeUI{}
		configRepo = testconfig.NewRepositoryWithDefaults()
		routingApiRepo = &fakeapi.FakeRoutingApiRepository{}
		repoLocator := deps.RepoLocator.SetRoutingApiRepository(routingApiRepo)

		domainRepo = &fakeapi.FakeDomainRepository{}
		repoLocator = repoLocator.SetDomainRepository(domainRepo)

		deps = command_registry.Dependency{
			Ui:          ui,
			Config:      configRepo,
			RepoLocator: repoLocator,
		}

		cmd = domain.CreateSharedDomain{}
		cmd.SetDependency(deps, false)

		flagContext = flags.NewFlagContext(cmd.MetaData().Flags)

		factory = &fakerequirements.FakeFactory{}

		loginRequirement = &passingRequirement{Name: "Login"}
		factory.NewLoginRequirementReturns(loginRequirement)

		routingApiRequirement = &passingRequirement{Name: "RoutingApi"}
		factory.NewRoutingAPIRequirementReturns(routingApiRequirement)

		minAPIVersionRequirement = &passingRequirement{}
		factory.NewMinAPIVersionRequirementReturns(minAPIVersionRequirement)

		routerGroups := models.RouterGroups{
			models.RouterGroup{
				Name: "router-group-name",
				Guid: "router-group-guid",
				Type: "router-group-type",
			},
		}
		routingApiRepo.ListRouterGroupsStub = func(cb func(models.RouterGroup) bool) error {
			for _, r := range routerGroups {
				if !cb(r) {
					break
				}
			}
			return nil
		}

	})

	Describe("Requirements", func() {
		Context("when not provided exactly one arg", func() {
			BeforeEach(func() {
				flagContext.Parse("arg-1", "extra-arg")
			})

			It("fails with usage", func() {
				Expect(func() { cmd.Requirements(factory, flagContext) }).To(Panic())
				Expect(ui.Outputs).To(ContainSubstrings(
					[]string{"Incorrect Usage. Requires DOMAIN as an argument"},
					[]string{"NAME"},
					[]string{"USAGE"},
				))
			})
		})

		Context("when provided exactly one arg", func() {
			BeforeEach(func() {
				flagContext.Parse("domain-name")
			})

			It("does not fail with usage", func() {
				Expect(ui.Outputs).NotTo(ContainSubstrings(
					[]string{"Incorrect Usage. Requires DOMAIN as an argument"},
					[]string{"NAME"},
					[]string{"USAGE"},
				))
			})

			It("returns a LoginRequirement", func() {
				actualRequirements, err := cmd.Requirements(factory, flagContext)
				Expect(err).NotTo(HaveOccurred())
				Expect(factory.NewLoginRequirementCallCount()).To(Equal(1))
				Expect(actualRequirements).To(ContainElement(loginRequirement))
			})

			Context("when router-group flag is set", func() {
				BeforeEach(func() {
					flagContext.Parse("domain-name", "--router-group", "route-group-name")
				})

				It("returns a LoginRequirement", func() {
					actualRequirements, err := cmd.Requirements(factory, flagContext)
					Expect(err).NotTo(HaveOccurred())
					Expect(factory.NewLoginRequirementCallCount()).To(Equal(1))
					Expect(actualRequirements).To(ContainElement(loginRequirement))
				})

				It("returns a RoutingApiRequirement", func() {
					actualRequirements, err := cmd.Requirements(factory, flagContext)
					Expect(err).NotTo(HaveOccurred())

					Expect(factory.NewRoutingAPIRequirementCallCount()).To(Equal(1))
					Expect(actualRequirements).To(ContainElement(routingApiRequirement))
				})

				It("returns a MinAPIVersionRequirement", func() {
					expectedVersion, err := semver.Make("2.36.0")
					Expect(err).NotTo(HaveOccurred())

					actualRequirements, err := cmd.Requirements(factory, flagContext)
					Expect(err).NotTo(HaveOccurred())

					Expect(factory.NewMinAPIVersionRequirementCallCount()).To(Equal(1))
					feature, requiredVersion := factory.NewMinAPIVersionRequirementArgsForCall(0)
					Expect(feature).To(Equal("Option '--router-group'"))
					Expect(requiredVersion).To(Equal(expectedVersion))
					Expect(actualRequirements).To(ContainElement(minAPIVersionRequirement))
				})
			})

			Context("when router-group flag is not set", func() {
				BeforeEach(func() {
					flagContext.Parse("domain-name")
				})

				It("returns a LoginRequirement", func() {
					actualRequirements, err := cmd.Requirements(factory, flagContext)
					Expect(err).NotTo(HaveOccurred())
					Expect(factory.NewLoginRequirementCallCount()).To(Equal(1))
					Expect(actualRequirements).To(ContainElement(loginRequirement))
				})

				It("does not return a RoutingApiRequirement", func() {
					actualRequirements, err := cmd.Requirements(factory, flagContext)
					Expect(err).NotTo(HaveOccurred())
					Expect(factory.NewRoutingAPIRequirementCallCount()).To(Equal(0))
					Expect(actualRequirements).ToNot(ContainElement(routingApiRequirement))
				})

				It("does not return a MinAPIVersionRequirement", func() {
					actualRequirements, err := cmd.Requirements(factory, flagContext)
					Expect(err).NotTo(HaveOccurred())
					Expect(actualRequirements).NotTo(ContainElement(minAPIVersionRequirement))
				})
			})
		})
	})

	Describe("Execute", func() {
		Context("when router-group flag is not set", func() {
			BeforeEach(func() {
				flagContext.Parse("domain-name")
				cmd.Execute(flagContext)
			})

			It("prints a message", func() {
				Expect(ui.Outputs).To(ContainSubstrings(
					[]string{"Creating shared domain domain-name"},
				))
			})

			It("creates a shared domain", func() {
				Expect(domainRepo.CreateSharedDomainCallCount()).To(Equal(1))

				domainName, routerGroupGuid := domainRepo.CreateSharedDomainArgsForCall(0)
				Expect(domainName).To(Equal("domain-name"))
				Expect(routerGroupGuid).To(Equal(""))
			})

			It("prints success message", func() {
				Expect(ui.Outputs).To(ContainSubstrings(
					[]string{"OK"},
				))
			})

		})

		Context("when cannot create shared domain", func() {
			BeforeEach(func() {
				flagContext.Parse("domain-name")
				domainRepo.CreateSharedDomainReturns(errors.New("create-domain-error"))
			})

			It("fails with error", func() {
				Expect(func() { cmd.Execute(flagContext) }).To(Panic())
				Expect(ui.Outputs).To(ContainSubstrings(
					[]string{"FAILED"},
					[]string{"create-domain-error"},
				))
			})
		})

		Context("when router-group flag is set", func() {
			BeforeEach(func() {
				flagContext.Parse("domain-name", "--router-group", "router-group-name")
			})

			It("retrieves a list of router groups from the Routing Api", func() {
				cmd.Execute(flagContext)
				Expect(routingApiRepo.ListRouterGroupsCallCount()).To(Equal(1))
			})

			Context("when routing group is found", func() {
				BeforeEach(func() {
					cmd.Execute(flagContext)
				})

				It("prints a message", func() {
					Expect(ui.Outputs).To(ContainSubstrings(
						[]string{"Creating shared domain domain-name"},
					))
				})

				It("creates a shared domain", func() {
					Expect(domainRepo.CreateSharedDomainCallCount()).To(Equal(1))

					domainName, routerGroupGuid := domainRepo.CreateSharedDomainArgsForCall(0)
					Expect(domainName).To(Equal("domain-name"))
					Expect(routerGroupGuid).To(Equal("router-group-guid"))
				})

				It("prints success message", func() {
					Expect(ui.Outputs).To(ContainSubstrings(
						[]string{"OK"},
					))
				})
			})

			Context("when ListRouterGroups returns an error", func() {
				BeforeEach(func() {
					routingApiRepo.ListRouterGroupsReturns(errors.New("router-group-error"))
				})

				It("fails with error message", func() {
					Expect(func() { cmd.Execute(flagContext) }).To(Panic())
					Expect(ui.Outputs).To(ContainSubstrings(
						[]string{"FAILED"},
						[]string{"router-group-error"},
					))
				})
			})
		})

		Context("when routing group is not found", func() {
			BeforeEach(func() {
				flagContext.Parse("domain-name", "--router-group", "router-group-name-1")
			})

			It("fails with a message", func() {
				Expect(func() { cmd.Execute(flagContext) }).To(Panic())
				Expect(ui.Outputs).To(ContainSubstrings(
					[]string{"FAILED"},
					[]string{"Router group router-group-name-1 not found"},
				))
			})

		})
	})
})
