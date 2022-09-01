NAME      = fluxdemo
PROVIDER  ?= open-component-model
IMAGE     = ocm$(NAME)
COMPONENT = github.com/$(PROVIDER)/ocm$(NAME)
OCMREPO   ?= ghcr.io/$(PROVIDER)/ocm
SIGNATURE ?= ocmfluxdemo
KEY       ?= 

REPO_ROOT                                      := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))/../..
VERSION                                        = $(shell git describe --tags --exact-match 2>/dev/null|| echo "$$(cat VERSION)-dev")
COMMIT                                         = $(shell git rev-parse HEAD)
EFFECTIVE_VERSION                              = $(VERSION)-$(COMMIT)

GEN = $(REPO_ROOT)/gen/$(NAME)
OCM = ocm

SRCS=$(shell find resources -type f)

.PHONY: ctf
ctf: $(GEN)/ctf

$(GEN)/ctf: $(GEN)/ca
	$(OCM) transfer ca $(GEN)/ca $(GEN)/ctf
	touch $(GEN)/ctf

.PHONY: plain-ctf
plain-ctf: $(GEN)
	$(OCM) transfer ca $(GEN)/ca $(GEN)/ctf
	touch $(GEN)/ctf

.PHONY: ca
ca: $(GEN)/ca

$(GEN)/ca: $(GEN) resources.yaml $(SRCS)
	$(OCM) create ca -f $(COMPONENT) "$(VERSION)" $(PROVIDER) $(GEN)/ca
	$(OCM) add resources $(GEN)/ca VERSION="$(VERSION)" COMMIT="$(COMMIT)" IMAGE="$(IMAGE):$(VERSION)" resources.yaml
	touch $(GEN)/ca


.PHONY: keys
keys: local/rsa.priv


local/rsa.priv:
	mkdir local
	$(OCM) create rsakeypair local/rsa.priv

.PHONY: sign
sign: $(GEN)/signed

$(GEN)/signed: $(GEN)/ctf $(KEY)
ifneq ($(KEY),)
	$(OCM) sign component -s $(SIGNATURE) -K $(KEY) $(GEN)/ctf
endif
	touch $(GEN)/signed

.PHONY: resign
resign: $(GEN)/ctf $(KEY)
ifneq ($(KEY),)
	$(OCM) sign component -s $(SIGNATURE) -K $(KEY) $(GEN)/ctf
endif
	touch $(GEN)/signed

.PHONY: verify
verify:
ifneq ($(KEY),)
	$(OCM) verify component -s $(SIGNATURE) -k $(KEY) $(OCMREPO)//$(COMPONENT):$(VERSION)
endif


.PHONY: push
push: $(GEN)/signed $(GEN)/push.$(NAME)

$(GEN)/push.$(NAME): $(GEN)/ctf
	$(OCM) transfer ctf -f $(GEN)/ctf $(OCMREPO)
	touch $(GEN)/push.$(NAME)

.PHONY: plain-push
plain-push: $(GEN)
	$(OCM) transfer ctf -f $(GEN)/ctf $(OCMREPO)
	touch $(GEN)/push.$(NAME)

$(GEN):
	mkdir -p $(GEN)

.PHONY: transport
transport:
ifneq ($(TARGETREPO),)
	$(OCM) transfer component -Vc  $(OCMREPO)//$(COMPONENT):$(VERSION) $(TARGETREPO)
endif



.PHONY: version
version:
	@echo $(VERSION)

.PHONY: info
info:
	@echo "ROOT:     $(REPO_ROOT)"
	@echo "VERSION:  $(VERSION)"
	@echo "COMMIT;   $(COMMIT)"



.PHONY: describe
describe: $(GEN)/ctf 
	ocm get resources -o treewide $(GEN)/ctf

.PHONY: descriptor
descriptor: $(GEN)/ctf 
	ocm get component -S v3alpha1 -o yaml $(GEN)/ctf



.PHONY: clean
clean:
	rm -rf $(GEN)
