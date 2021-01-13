#!/bin/bash

# Copyright 2020 Authors of Arktos.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

function print_latency {
    printf "\n ----------------- Latency -----------------------\n"
    for tenant in ${tenants}    
    do 
        if [[ -d ${tenant} ]]; then
            printf " Tenant ${tenant}:\n"
            printf "   saturation pods: " && grep perc99 ${tenant}/*log | grep threshold | sed -n 1p |awk -F " " '{print $6,$7,$8,$9,$10,$11}' 
            printf "   latency pods: " && grep perc99 ${tenant}/*log | grep threshold | sed -n 2p |awk -F " " '{print $6,$7,$8,$9,$10,$11}'
        fi        
    done
}

function print_throughput {
    printf "\n ----------------- Throughput -----------------------\n"
    for tenant in ${tenants}    
    do 
        if [[ -d ${tenant} ]]; then
            printf "Tenant ${tenant}:"
            cat ${tenant}/SchedulingThroughput_density*  | tr -d '\n{}\"'
            echo
        fi        
    done
}

tenants=${TENANTS:-"arktos margaret ryan zeta"}

if [[ -d "perf-log" ]]; then cd perf-log; fi

print_latency

print_throughput

echo
