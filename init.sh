#!/bin/bash

ROOTPATH="${1}"

sqlite3 "${ROOTPATH}/bank.db" < bank/accounts/schema.sql
