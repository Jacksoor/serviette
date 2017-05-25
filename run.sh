#!/bin/bash
trap 'killall' INT

killall() {
    kill -INT 0
    wait
}

ROOTPATH="${1}"
TOKEN="${2}"

bank/bank -sqlite_db_path="${ROOTPATH}/bank.db" -logtostderr &
executor/executor -scripts_root_path="${ROOTPATH}/scripts" -images_root_path="${ROOTPATH}/images" -logtostderr &
discordbridge/discordbridge -currency_name="<:cummies2:315911986976129024>" -status="dev" -discord_token="${TOKEN}" -logtostderr &
webbridge/webbridge -logtostderr &

socat -d -d TCP-LISTEN:1234,reuseaddr,fork UNIX-CLIENT:/tmp/kobun4-webbridge.socket &

wait
