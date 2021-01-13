#!/usr/bin/env bash

echo "To see live Prometheus: "
export GCE_PROJECT=${GCE_PROJECT:-"workload-controller-manager"}
export GCE_REGION=${GCE_REGION:-"us-central1-b"}
export RUN_PREFIX=${RUN_PREFIX:-"qian-verify"}
export SCALEOUT_TP_COUNT="${SCALEOUT_TP_COUNT:-1}"

function get_ip_addr {
    instance_name=$1
    IP_ADDR=$(gcloud compute instances describe ${instance_name} --zone "${GCE_REGION}" --project "${GCE_PROJECT}"  --format='get(networkInterfaces[0].accessConfigs[0].natIP)')
    
}

for (( tp_num=1; tp_num<=${SCALEOUT_TP_COUNT}; tp_num++ ))
do
  get_ip_addr ${RUN_PREFIX}-kubemark-tp-${tp_num}-master
  printf " TP-${tp_num} : http://${IP_ADDR}:9090\n"
done

get_ip_addr ${RUN_PREFIX}-kubemark-rp-master
printf " RP : http://${IP_ADDR}:9090\n"
