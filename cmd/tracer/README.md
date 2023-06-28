### How to run tracer example locally.



In terminal run: 
```sh
make builder-image-enter
```

Now you can build bpf code and run tracer.

```sh
make gen-bpf && go run ./cmd/tracer/
```

In separate terminal exec in builder container and trigger some dns requests.

```sh
docker exec -it egressd-ebpf-builder /bin/bash
curl google.com
```

You should now see response in first terminal

```
Questions:
IN A google.com 
Answers:
IN A google.com [142.250.74.78] []
```
