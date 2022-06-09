# Kube events tailer

List and tail all events in cluster. Just a simple app to help me learn the Kubernetes go-client library.

## Usage

```shell-session
$ ./k8s-event-tailer -h
usage: k8s-event-tailer [<flags>]

Flags:
  -h, --help               Show context-sensitive help (also try --help-long and --help-man).
  -k, --kubeconfig="~/.kube/config"  
                           Path to kubeconfig or set in env(KUBECONFIG)
  -v, --verbose            Debug logging
  -n, --namespace=""       Namespace
  -s, --stats-interval=10  Seconds after which stats are printed

```

Prints some stats after every 10 seconds by default. Turn it off by setting `--stats-interval` to `0`.

If no namespace mentioned, will list events in all namespaces.

## Sample output

```text
2022-06-10T00:10:23+02:00 INF Event added count=1 eventMsg="Successfully assigned default/nginx to minikube" lastTimestamp=2022-06-09T21:53:12Z name=nginx.16f7126032cf0392 namespace=default version=540352
2022-06-10T00:10:23+02:00 INF Event added count=1 eventMsg="Pulling image \"nginx\"" lastTimestamp=2022-06-09T21:53:14Z name=nginx.16f71260b99cc852 namespace=default version=540356
2022-06-10T00:10:23+02:00 INF Event added count=1 eventMsg="Successfully pulled image \"nginx\" in 1.421922703s" lastTimestamp=2022-06-09T21:53:16Z name=nginx.16f712610e5dbb93 namespace=default version=540358
2022-06-10T00:10:23+02:00 INF Event added count=1 eventMsg="Created container nginx" lastTimestamp=2022-06-09T21:53:16Z name=nginx.16f7126113a60fb0 namespace=default version=540359
2022-06-10T00:10:23+02:00 INF Event added count=1 eventMsg="Started container nginx" lastTimestamp=2022-06-09T21:53:16Z name=nginx.16f712612453d99d namespace=default version=540360
2022-06-10T00:10:24+02:00 INF STATS: Number of items in store: 7
2022-06-10T00:10:24+02:00 INF STATS: addCounter: 7, updateCounter: 0, deleteCounter: 0
2022-06-10T00:10:24+02:00 INF STATS: added: 7, updated: 0, deleted: 0
```
