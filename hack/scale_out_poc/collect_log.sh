#!/usr/bin/env bash

echo "start collecting testing logs "
export GCE_PROJECT=${GCE_PROJECT:-"workload-controller-manager"}
export GCE_REGION=${GCE_REGION:-"us-central1-b"}
export RUN_PREFIX=${RUN_PREFIX:-"qian-verify"}
export SCALEOUT_TP_COUNT="${SCALEOUT_TP_COUNT:-1}"


function generate_remotelogs {
    local region=${1:-${GCE_REGION}}
    local project=${2:-${GCE_PROJECT}}
    local name=${3:-${MACHINE_NAME}}
    echo "generating dmesg.log, kubelet.log, journalctl.log, prometheus snapshot on remote machine:$name"
    gcloud beta compute ssh --zone "${region}" --project "${project}" "${name}" --command="sudo dmesg >> dmesg.log && sudo journalctl -u kubelet >> kubelet.log && sudo journalctl --since \"$(date -d "2 days ago" +"%Y-%m-%d 00:00:01")\" --until \"$(date +'%Y-%m-%d %H:%M:%S')\" >> journalctl.log"
    gcloud beta compute ssh --zone "${region}" --project "${project}" "${name}"  --command="curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot"
}

function copylogs {
    local region=${1:-${GCE_REGION}}
    local project=${2:-${GCE_PROJECT}}
    local name=${3:-${MACHINE_NAME}}
    local location=${4:-"./"}
    echo "copying /var/log and prometheus snapshot from remote machine:$name to $location"
    gcloud beta compute scp --recurse --zone "${region}" --project "${project}" "${name}":/var/log ${location}
    gcloud beta compute scp --zone "${region}" --project "${project}" "${name}":dmesg.log ${location}
    gcloud beta compute scp --zone "${region}" --project "${project}" "${name}":kubelet.log ${location}
    gcloud beta compute scp --zone "${region}" --project "${project}" "${name}":journalctl.log ${location}
    if [[ ! -e ${location}prometheus ]]; then
        mkdir ${location}prometheus
    fi
    cd ${location}prometheus
    gcloud beta compute scp --recurse --zone "${region}" --project "${project}" "${name}":/etc/srv/kubernetes/prometheus-2.2.1.linux-amd64/data/snapshots ./
    zip -r snapshots.zip ./snapshots
    cd ..
}

function copyminionlogs {
    local region=${1:-${GCE_REGION}}
    local project=${2:-${GCE_PROJECT}}
    local name=${3:-${MACHINE_NAME}}
    local location=${4:-"./"}
    echo "copying hollow-node logs from remote machine:$name to $location"
    gcloud beta compute scp --zone "${region}" --project "${project}" --tunnel-through-iap "${name}":dmesg.log ${location}
    gcloud beta compute scp --zone "${region}" --project "${project}" --tunnel-through-iap "${name}":kubelet.log ${location}
    gcloud beta compute scp --zone "${region}" --project "${project}" --tunnel-through-iap "${name}":journalctl.log ${location}
    gcloud beta compute scp --zone "${region}" --project "${project}" --tunnel-through-iap "${name}":/var/log/*hollow-node-z* ${location}
}

function collect_logs {
    dir=$1
    specific_type=$2
    pushd .

    if [[ ! -e $dir ]]; then
        mkdir $dir
    elif [[ ! -d $dir ]]; then
        echo "$dir already exists but is not a directory" 1>&2
        exit
    fi
    cd $dir

    generate_remotelogs
    if [[ "${specific_type}" == "minion"]]; then
        copyminionlogs
    else
        copylogs
    fi

    popd
}

### collect admin master logs
MACHINE_NAME="${RUN_PREFIX}-master"
collect_logs "admin_master"

### collect minion hollow-nodes logs
MACHINE_NAME=$(gcloud compute instance-groups list-instances ${RUN_PREFIX}-minion-group --zone "${GCE_REGION}" --project "${GCE_PROJECT}"  | awk 'FNR == 2 {print $1}')
collect_logs "minion-group-${MACHINE_NAME##*-}" "minion"

#collect TP logs
for (( tp_num=1; tp_num<=${SCALEOUT_TP_COUNT}; tp_num++ ))
do
  MACHINE_NAME=$(gcloud compute instance-groups list-instances ${RUN_PREFIX}-tp-${tp_num}-master --zone "${GCE_REGION}" --project "${GCE_PROJECT}"  | awk 'FNR == 2 {print $1}')
  collect_logs "tp${tp_num}-master"
done

# collect RP logs
MACHINE_NAME=$(gcloud compute instance-groups list-instances ${RUN_PREFIX}-rp-master --zone "${GCE_REGION}" --project "${GCE_PROJECT}"  | awk 'FNR == 2 {print $1}')
collect_logs "rp-master"