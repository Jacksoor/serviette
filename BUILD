load("@io_bazel_rules_go//go:def.bzl", "go_prefix")
load("@bazel_tools//tools/build_defs/pkg:pkg.bzl", "pkg_tar", "pkg_deb")

go_prefix("github.com/porpoises/kobun4")

pkg_tar(
    name = "kobun4_dist",
    extension = "tar.gz",
    files = [
        "//clients",
        "//delegator/supervisor",
        "//discordbridge",
        "//discordbridge:schema.sql",
        "//discordbridge/web",
        "//discordbridge/web/static",
        "//executor",
        "//executor:schema.sql",
        "//restbridge",
        "//systemd",
    ],
    modes = {
        "//discordbridge": "0755",
        "//discordbridge/web": "0755",
        "//executor": "0755",
        "//restbridge": "0755",
    },
    package_dir = "kobun4",
    strip_prefix = "/",
)
