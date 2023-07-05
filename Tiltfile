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

if os.environ['TILT_DEFAULT_REGISTRY']:
    default_registry(os.environ['TILT_DEFAULT_REGISTRY'])

namespace_create(namespace)


go_env = {
    'CGO_ENABLED': '0',
    'GOOS': 'linux',
    'GOARCH': 'amd64',
}
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


k8s_yaml('./hack/network-test-app.yaml')
