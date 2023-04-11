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

namespace_create(namespace)

local_resource(
    'egressd-compile',
    'CGO_ENABLED=0 GOOS=linux go build -o ./bin/egressd ./cmd/collector',
    deps=[
        './'
    ],
    ignore=[
        './bin',
    ],
)

local_resource(
    'egressd-exporter-compile',
    'CGO_ENABLED=0 GOOS=linux go build -o ./bin/egressd-exporter ./cmd/exporter',
    deps=[
        './'
    ],
    ignore=[
        './bin',
    ],
)

docker_build_with_restart(
    'localhost:5000/egressd',
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
    'localhost:5000/egressd-exporter',
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

helm_remote(
    'grafana',
    repo_url='https://grafana.github.io/helm-charts',
    repo_name='grafana',
    version='6.50.7',
    namespace=namespace,
    set=[],
    values=['./hack/grafana-tilt-values.yaml']
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

k8s_yaml('./hack/network-test-app.yaml')
