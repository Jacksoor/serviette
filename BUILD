load("@io_bazel_rules_go//go:def.bzl", "go_prefix")
load("@bazel_tools//tools/build_defs/pkg:pkg.bzl", "pkg_tar", "pkg_deb")

go_prefix("github.com/porpoises/kobun4")

pkg_tar(
    name = "kobun4_dist",
    extension = "tar.gz",
    files = [
        "//clients",
        "//discordbridge",
        "//discordbridge/varstore:schema.sql",
        "//executor",
        "//executor/accounts:schema.sql",
        "//restbridge",
        "//systemd",
        "//webbridge",
        "//webbridge/static",
        "//webbridge/templates",
    ],
    modes = {
        "//discordbridge": "0755",
        "//executor": "0755",
        "//restbridge": "0755",
        "//webbridge": "0755",
    },
    package_dir = "kobun4",
    strip_prefix = "/",
)
