load("@io_bazel_rules_go//go:def.bzl", "go_prefix")
load("@bazel_tools//tools/build_defs/pkg:pkg.bzl", "pkg_tar", "pkg_deb")

go_prefix("github.com/porpoises/kobun4")

sh_binary(
    name = "run",
    srcs = ["run.sh"],
    data = [
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
    ],
    visibility = ["//visibility:public"],
)

pkg_tar(
    name = "kobun4_dist",
    extension = "tar.gz",
    files = [
        ":init.sh",
        "//clients",
        "//discordbridge",
        "//discordbridge/varstore:schema.sql",
        "//executor",
        "//executor/accounts:schema.sql",
        "//webbridge",
        "//webbridge/static",
        "//webbridge/templates",
        "//systemd",
    ],
    modes = {
        ":init.sh": "0755",
        "//discordbridge": "0755",
        "//executor": "0755",
        "//webbridge": "0755",
    },
    package_dir = "kobun4",
    strip_prefix = "/",
)
