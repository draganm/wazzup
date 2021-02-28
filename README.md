# wazzup

Small k8s utility that will tail logs and events from all pods (all containers in each of the pods, including the init containers) belonging to a specified deployment 
This can be very useful when trying to pinpoint issues with pods not starting.

## Building

```bash
go build . -o wazzup
```

### Use

```bash
wazzup -n <k8s namespace> <deployment name>
```
