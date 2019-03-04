# Copyright 2018 Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may
# not use this file except in compliance with the License. A copy of the
# License is located at
#
# 	http://aws.amazon.com/apache2.0/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

# Set this to pass additional commandline flags to the go compiler, e.g. "make test EXTRAGOARGS=-v"
EXTRAGOARGS:=

all: build

test: all-tests

unit-tests:
	go test -short ./... $(EXTRAGOARGS)

all-tests:
	go test ./... $(EXTRAGOARGS)

generate build clean:
	go $@ $(EXTRAGOARGS)

sandbox-test-fc-build:
	docker build -f fctesting/sandbox/Dockerfile -t "firecracker" .
	@touch sandbox-test-fc-build

sandbox-test-fc-run:
	docker run \
		--init \
		--rm \
		--privileged \
		--security-opt seccomp=unconfined \
		--ulimit core=0 \
		--device=/dev/kvm:/dev/kvm \
		-t firecracker

sandbox-test-fc: sandbox-test-fc-build sandbox-test-fc-run

.PHONY: all generate clean build test unit-tests all-tests
