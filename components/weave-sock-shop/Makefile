NAME                = ocm-sock-shop
PROVIDER            ?= open-component-model
WW_DEMOS_PROVIDER   ?= weaveworksdemos
IMAGE               = $(NAME)
COMPONENT           = github.com/$(PROVIDER)/$(NAME)
OCI_REPO_BASE       ?= ghcr.io/$(PROVIDER)
OCI_REPO            ?= $(OCMREPO_BASE)/ocm-sock-shop
KEY                 ?= rsa.priv
SIGN_NAME           ?= ci-keypair

CHILD_COMPONENTS    ?= carts catalogue front-end namespace orders payment queue-master shipping user
GEN_DIRS            := $(addprefix $(CHILD_COMPONENTS),/gen)

VERSION             := $(shell git describe --tags --exact-match 2>/dev/null|| echo "$$(cat VERSION)-dev")
COMMIT              = $(shell git rev-parse HEAD)
EFFECTIVE_VERSION   = $(VERSION)-$(COMMIT)

OCM = ocm

define make-child

.PHONY: $1
$1: $1/gen $1/ca $1/push

$1/ca:
	$(OCM) create ca -f github.com/$(WW_DEMOS_PROVIDER)/$1 $(shell cat $1/VERSION) --provider $(WW_DEMOS_PROVIDER) -F $1/gen/ca --scheme ocm.software/v3alpha1
	$(OCM) add resources $1/gen/ca $1/resources.yaml

$1/push:
	$(OCM) transfer component -f --copy-resources $1/gen/ca $(OCI_REPO_BASE)/$1

$1/sign: keypair
	$(OCM) sign component -s $(SIGN_NAME) -K $(KEY) --repo $(OCI_REPO_BASE) github.com/$(WW_DEMOS_PROVIDER)/$1

$1/gen:
	@mkdir -p $1/gen

$1/clean:
	rm -rf $1/gen

endef

.PHONY: all
all: $(CHILD_COMPONENTS) push

ca: gen
	$(OCM) create ca -f $(COMPONENT) $(VERSION) --provider $(PROVIDER) -F gen/ca --scheme ocm.software/v3alpha1
	$(OCM) add references gen/ca references.yaml

push:
	$(OCM) transfer component -f --copy-resources --recursive --lookup $(OCI_REPO) gen/ca $(OCI_REPO)

keypair:
	$(OCM) create rsakeypair

sign:
	$(OCM) sign component -s $(SIGN_NAME) -K $(KEY) --repo $(OCI_REPO_BASE) $(COMPONENT)

gen:
	@mkdir -p gen

$(GEN_DIRS):
	@mkdir -p $@

clean:
	rm -rf gen

clean-all: clean $(CHILD_COMPONENTS).clean

$(foreach cdir,$(CHILD_COMPONENTS),$(eval $(call make-child,$(cdir))))