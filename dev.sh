#!/bin/bash
# Copyright 2024 The packetd Authors
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


set -e

function push() {
  rsync -avz --delete --exclude='.idea/' --exclude='.git/' ../packetd fedora:/root/projects/golang/
}

function pull() {
  rsync fedora:/root/projects/golang/packetd/sniffer/libebpf/*bpfe* ./sniffer/libebpf/
}


function run() {
  go build . && ./packetd agent --config ./packetd.yaml
}

if [ "$1" == "push" ]; then
  push
elif [ "$1" == "pull" ]; then
  pull
elif [ "$1" == "run" ]; then
  run
fi
