load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["cluster-connector.go"],
    importpath = "k8s.io/kubernetes/cmd/cluster-connector",
    visibility = ["//visibility:private"],
    deps = [
        "//cmd/cluster-connector/app:go_default_library",
        "//staging/src/k8s.io/arktos-ext/pkg/generated/clientset/versioned:go_default_library",
        "//staging/src/k8s.io/arktos-ext/pkg/generated/informers/externalversions:go_default_library",
        "//staging/src/k8s.io/client-go/rest:go_default_library",
        "//staging/src/k8s.io/client-go/tools/cache:go_default_library",
        "//staging/src/k8s.io/client-go/tools/clientcmd:go_default_library",
        "//vendor/k8s.io/klog:go_default_library",
    ],
)

go_binary(
    name = "arktos-network-controller",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//cmd/cluster-connector/app:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
