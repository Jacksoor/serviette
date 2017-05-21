#!/bin/bash
trap 'killall' INT

killall() {
    kill -INT 0
    wait
}

ROOTPATH="${1}"
TOKEN="${2}"

bank/bank -sqlite_db_path="${ROOTPATH}/bank.db" -logtostderr &
discordbridge/discordbridge -discord_token="${TOKEN}" -status="C.R.E.A.M." -currency_name="<:cummies2:315911986976129024>" -logtostderr &
executor/executor -script_root_path="${ROOTPATH}/scripts" -nsjail_path="${ROOTPATH}/nsjail" -logtostderr &
webbridge/webbridge -logtostderr &

wait
