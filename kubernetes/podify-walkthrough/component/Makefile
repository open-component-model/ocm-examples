NAME                = podify
PROVIDER            ?= weaveworks
COMPONENT           = github.com/$(PROVIDER)/$(NAME)
OCI_REPO            ?= ghcr.io/phoban01

CHILD_COMPONENTS    ?= backend frontend cache
GEN_DIRS            := $(addprefix $(CHILD_COMPONENTS),/gen)

VERSION             := $(shell cat VERSION)

OCM = ocm

define make-child

.PHONY: $1
$1: $1/clean $1/gen $1/ca $1/push

$1/gen:
	mkdir -p $1/gen

$1/ca:
	$(OCM) create componentarchive github.com/$(PROVIDER)/$1 $(shell cat $1/VERSION) \
		--provider $(PROVIDER) --file $1/gen/ca --scheme ocm.software/v3alpha1 -f
	$(OCM) add resources $1/gen/ca $1/resources.yaml

$1/push: $1/ca
	$(OCM) transfer component -f $1/gen/ca $(OCI_REPO)

$1/clean:
	rm -rf $1/gen

endef

.PHONY: all
all: $(CHILD_COMPONENTS) push sign

keys:
	rm -f key.priv key.pub && \
		$(OCM) create rsakeypair key.priv key.pub

sign: keys
	$(OCM) sign component -s default -K key.priv --repo $(OCI_REPO) $(COMPONENT)

ca: gen
	$(OCM) create ca -f $(COMPONENT) $(VERSION) --provider $(PROVIDER) -F gen/ca --scheme ocm.software/v3alpha1
	$(OCM) add references gen/ca references.yaml

push: ca
	$(OCM) transfer component -f gen/ca $(OCI_REPO)

push-copy: ca
	$(OCM) transfer component -f --copy-resources --recursive --lookup $(OCI_REPO) gen/ca $(OCI_REPO)

gen:
	@mkdir -p gen

clean:
	rm -rf gen

clean-all: clean $(CHILD_COMPONENTS).clean

$(foreach cdir,$(CHILD_COMPONENTS),$(eval $(call make-child,$(cdir))))
