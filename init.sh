#!/bin/bash

ROOTPATH="${1}"

sqlite3 "${ROOTPATH}/bank.db" < bank/accounts/schema.sql
mkdir "${ROOTPATH}/scripts"
mkdir "${ROOTPATH}/images"
sqlite3 "${ROOTPATH}/executor.db" < executor/scripts/schema.sql
