#!/usr/bin/env bash

echo "To see live Prometheus: "
export GCE_PROJECT=${GCE_PROJECT:-"workload-controller-manager"}
export GCE_REGION=${GCE_REGION:-"us-central1-b"}
export RUN_PREFIX=${RUN_PREFIX:-"qian-verify"}
export SCALEOUT_TP_COUNT="${SCALEOUT_TP_COUNT:-1}"

function get_ip_addr {
    instance_name=$1
    IP_ADDR=$(gcloud compute instances describe ${instance_name} --zone "${GCE_REGION}" --project "${GCE_PROJECT}"  --format='get(networkInterfaces[0].accessConfigs[0].natIP)')
    printf " ${instance_name} : ${IP_ADDR}\n"
}

for (( tp_num=1; tp_num<=${SCALEOUT_TP_COUNT}; tp_num++ ))
do
  get_ip_addr ${RUN_PREFIX}-kubemark-tp-${tp_num}-master
done

get_ip_addr ${RUN_PREFIX}-kubemark-rp-master

get_ip_addr ${RUN_PREFIX}-master

get_ip_addr ${RUN_PREFIX}-kubemark-proxy

for minion in $(gcloud compute instance-groups list-instances ${RUN_PREFIX}-minion-group --zone "${GCE_REGION}" --project "${GCE_PROJECT}" | awk 'NR>1 {print $1}')
do 
  echo "------------ $minion"
  get_ip_addr $minion
done
