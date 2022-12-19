load('ext://restart_process', 'docker_build_with_restart')
load('ext://namespace', 'namespace_create')
load('ext://dotenv', 'dotenv')

if read_file ('.env' , default = '' ):
    dotenv()

update_settings(max_parallel_updates=16)
secret_settings ( disable_scrub = True)
allow_k8s_contexts(['tilt', 'kind-tilt', 'docker-desktop', 'minikube'])
namespace = 'egressd'
user = os.environ.get('USER', 'unknown-user')

namespace_create(namespace)

local_resource(
    'egressd-compile',
    'CGO_ENABLED=0 GOOS=linux go build -o ./bin/egressd ./',
    deps=[
        './'
    ],
    ignore=[
        './bin',
    ],
)

local_resource(
    'egressd-docker-build',
    'docker build -t localhost:5000/egressd . -f Dockerfile && docker push localhost:5000/egressd',
    deps=[
        './bin/egressd'
    ],
)

docker_build_with_restart(
    'localhost:5000/egressd',
    '.',
    entrypoint=['/usr/local/bin/egressd'],
    dockerfile='Dockerfile',
    only=[
        './bin/egressd',
    ],
    live_update=[
        sync('./bin/egressd', '/usr/local/bin/egressd'),
    ],
)

chart_path = './charts/egressd'
k8s_yaml(helm(
    chart_path,
    name='egressd',
    namespace=namespace,
    set=[
      'image.repository=localhost:5000/egressd'
    ]
))
