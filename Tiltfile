load('ext://restart_process', 'docker_build_with_restart')
load('ext://helm_remote', 'helm_remote')
load('ext://namespace', 'namespace_create')
load('ext://dotenv', 'dotenv')

if read_file ('.env' , default = '' ):
    dotenv()

update_settings(max_parallel_updates=16)
secret_settings (disable_scrub = True)
allow_k8s_contexts(['tilt', 'kind-tilt', 'docker-desktop', 'minikube'])
namespace = 'egressd'
user = os.environ.get('USER', 'unknown-user')

if os.environ.get('TILT_DEFAULT_REGISTRY'):
    default_registry(os.environ['TILT_DEFAULT_REGISTRY'])

namespace_create(namespace)


go_env = {
    'CGO_ENABLED': '0',
    'GOOS': 'linux',
    'GOARCH': 'amd64',
}

# Allow to override architecture, e.g. when building on M1,
# but running on GKE.
goarch = os.environ.get('TILT_GOARCH')
if goarch:
    go_env['GOARCH'] = goarch

local_resource(
    'egressd-compile',
    'go build -o ./bin/egressd ./cmd/collector',
    env=go_env,
    deps=[
        './'
    ],
    ignore=[
        './bin',
    ],
)

local_resource(
    'egressd-exporter-compile',
    'go build -o ./bin/egressd-exporter ./cmd/exporter',
    env=go_env,
    deps=[
        './'
    ],
    ignore=[
        './bin',
    ],
)

docker_build_with_restart(
    'egressd',
    '.',
    entrypoint=['/usr/local/bin/egressd'],
    dockerfile='Dockerfile.tilt',
    only=[
        './bin/egressd',
    ],
    live_update=[
        sync('./bin/egressd', '/usr/local/bin/egressd'),
    ],
)

docker_build_with_restart(
    'egressd-exporter',
    '.',
    entrypoint=['/usr/local/bin/egressd-exporter'],
    dockerfile='Dockerfile.exporter.tilt',
    only=[
        './bin/egressd-exporter',
    ],
    live_update=[
        sync('./bin/egressd-exporter', '/usr/local/bin/egressd-exporter'),
    ],
)

chart_path = './charts/egressd'

k8s_yaml(helm(
    chart_path,
    name='egressd',
    namespace=namespace,
    values=['./charts/egressd/values-tilt.yaml']
))

if os.environ.get("DISABLE_METRICS", "false") != "false":
    helm_remote(
        'grafana',
        repo_url='https://grafana.github.io/helm-charts',
        repo_name='grafana',
        version='6.50.7',
        namespace=namespace,
        set=[],
        values=['./hack/grafana-tilt-values.yaml']
    )
    k8s_resource(
      workload='grafana',
      port_forwards=[
        port_forward(name="grafana", local_port=3334, container_port=3000),
      ],
    )

    helm_remote(
      'loki',
      repo_url='https://grafana.github.io/helm-charts',
      repo_name='grafana',
      release_name='loki',
      namespace=namespace,
      version='5.2.0',
      values=['./hack/loki-tilt-values.yaml'],
      set=[]
    )
    k8s_resource(
      workload='loki',
      port_forwards=[
        port_forward(name="ui", local_port=3101, container_port=3100),
      ],
    )

    helm_remote(
        'victoria-metrics-single',
        repo_url='https://victoriametrics.github.io/helm-charts',
        repo_name='victoria',
        version='0.8.58',
        namespace=namespace,
        set=[],
        values=[]
    )

    helm_remote(
      'promtail',
      repo_url='https://grafana.github.io/helm-charts',
      repo_name='grafana',
      release_name='promtail',
      namespace=namespace,
      version='3.11.0',
      values=['./hack/promtail-values.yaml'],
      set=[]
    )



k8s_yaml('./hack/network-test-app.yaml')
