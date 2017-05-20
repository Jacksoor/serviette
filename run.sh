#!/bin/bash
trap 'killall' INT

killall() {
    kill -INT 0
    wait
}

ROOTPATH="${1}"
TOKEN="${2}"

bank/bank -sqlite_db_path="${ROOTPATH}/bank.db" -logtostderr &
discordbridge/discordbridge -discord_token="${TOKEN}" -status="C.R.E.A.M." -currency_name="<:cummies:315372705933295618>" -sqlite_db_path="${ROOTPATH}/discordbridge.db" -logtostderr &
executor/executor -logtostderr &
webbridge/webbridge -logtostderr &

wait
