load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["controller.go"],
    importpath = "k8s.io/kubernetes/cmd/cluster-connector/app",
    visibility = ["//visibility:public"],
    deps = [
        "//staging/src/k8s.io/apimachinery/pkg/apis/meta/v1:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/labels:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/util/runtime:go_default_library",
        "//staging/src/k8s.io/apimachinery/pkg/util/wait:go_default_library",
        "//staging/src/k8s.io/arktos-ext/pkg/apis/arktosedgeextensions/v1:go_default_library",
        "//staging/src/k8s.io/arktos-ext/pkg/generated/clientset/versioned/scheme:go_default_library",
        "//staging/src/k8s.io/arktos-ext/pkg/generated/informers/externalversions/arktosedgeextensions/v1:go_default_library",
        "//staging/src/k8s.io/arktos-ext/pkg/generated/listers/arktosedgeextensions/v1:go_default_library",
        "//staging/src/k8s.io/client-go/kubernetes/scheme:go_default_library",
        "//staging/src/k8s.io/client-go/tools/cache:go_default_library",
        "//staging/src/k8s.io/client-go/util/workqueue:go_default_library",
        "//vendor/k8s.io/klog:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
