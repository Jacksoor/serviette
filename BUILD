load("@io_bazel_rules_go//go:def.bzl", "go_prefix")

go_prefix("github.com/porpoises/kobun4")

sh_binary(
    name = "run",
    srcs = ["run.sh"],
    data = [
        "//bank",
        "//discordbridge",
        "//executor",
        "//webbridge",
    ],
    visibility = ["//visibility:public"],
)

sh_binary(
    name = "init",
    srcs = ["init.sh"],
    data = [
        "//bank/accounts:schema.sql",
    ],
    visibility = ["//visibility:public"],
)
