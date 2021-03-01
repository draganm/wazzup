package main

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/draganm/wazzup/logwriter"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	a := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "namespace",
				EnvVars: []string{"NAMESPACE"},
				Aliases: []string{"n"},
				Value:   "default",
			},
			&cli.StringFlag{
				Name:    "kubeconfig",
				EnvVars: []string{"KUBECONFIG"},
			},
		},
		Action: func(c *cli.Context) error {

			if c.Args().Len() != 1 {
				return errors.New("deployment name must be provided")
			}

			deploymentName := c.Args().First()

			config, err := clientcmd.BuildConfigFromFlags("", c.String("kubeconfig"))
			if err != nil {
				return errors.Wrap(err, "while creating k8s cluster config")
			}

			kubeclient, err := kubernetes.NewForConfig(config)
			if err != nil {
				return errors.Wrap(err, "while creating k8s client")
			}

			pods := newPodsByName()

			lines := make(chan string)

			evtclient := kubeclient.CoreV1().Events(c.String("namespace"))
			evtWatch, err := evtclient.Watch(context.Background(), v1.ListOptions{})
			if err != nil {
				return errors.Wrap(err, "while creating event watch")
			}

			deploymentsClient := kubeclient.AppsV1().Deployments(c.String("namespace"))
			depl, err := deploymentsClient.Get(context.Background(), deploymentName, v1.GetOptions{})
			if err != nil {
				return errors.Wrap(err, "while getting deployment")
			}

			podSelector := depl.Spec.Selector

			go func() {
				for e := range evtWatch.ResultChan() {
					evt := e.Object.(*corev1.Event)
					if evt.InvolvedObject.Kind == "Pod" && pods.contains(evt.InvolvedObject.Name) {
						lines <- fmt.Sprintf("EVENT %s(%s): %s", evt.InvolvedObject.Kind, evt.InvolvedObject.Name, evt.Message)
					}

				}
			}()

			podsClient := kubeclient.CoreV1().Pods(c.String("namespace"))

			podWatch, err := podsClient.Watch(context.Background(), v1.ListOptions{
				LabelSelector: v1.FormatLabelSelector(podSelector),
				Watch:         true,
			})
			if err != nil {
				return errors.Wrap(err, "while creating pod watch")
			}

			defer podWatch.Stop()

			go func() {

			podWatchLoop:
				for evt := range podWatch.ResultChan() {
					pod, castOk := evt.Object.(*corev1.Pod)
					if !castOk {
						continue podWatchLoop
					}

					podName := pod.ObjectMeta.Name
					switch evt.Type {
					case watch.Added:
						pods.set(podName, pod)
						if evt.Type == watch.Added {
							lines <- fmt.Sprintf("POD %s created", podName)
						}
						statuses := append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...)
						for _, c := range statuses {

							shouldTailLogs := c.State.Waiting == nil
							if !shouldTailLogs {
								continue
							}
							containerName := c.Name
							tailLines := int64(30)
							containerLogs := podsClient.GetLogs(podName, &corev1.PodLogOptions{Container: containerName, Follow: true, TailLines: &tailLines})
							go func() {
								st, err := containerLogs.Stream(context.Background())
								if err != nil {
									lines <- fmt.Sprintf("error while getting logs for container %s of pod %s: %s", containerName, podName, err.Error())
									return
								}
								defer st.Close()
								lw := logwriter.New(func(lns []string) error {
									for _, l := range lns {
										lines <- fmt.Sprintf("LOG(%s/%s): %s", podName, containerName, l)
									}
									return nil
								})
								_, err = io.Copy(lw, st)
								if err != nil {
									lines <- fmt.Sprintf("error while reading/writing logs for container %s of pod %s: %s", containerName, podName, err.Error())
								}
								lines <- fmt.Sprintf("END OF LOG: container %s in pod %s terminated", podName, containerName)
							}()

						}
						continue podWatchLoop
					case watch.Modified:
						if !pods.contains(podName) {
							continue podWatchLoop
						}
						old := pods.get(podName)
						pods.set(podName, pod)

						oldStatuses := append(old.Status.InitContainerStatuses, old.Status.ContainerStatuses...)
						newStatuses := append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...)

						for i, oldStatus := range oldStatuses {
							newStatus := newStatuses[i]

							if newStatus.State.Waiting == nil && oldStatus.State.Waiting != nil {
								containerName := newStatus.Name
								tailLines := int64(30)
								containerLogs := podsClient.GetLogs(podName, &corev1.PodLogOptions{Container: containerName, Follow: true, TailLines: &tailLines})
								go func() {
									st, err := containerLogs.Stream(context.Background())
									if err != nil {
										lines <- fmt.Sprintf("error while getting logs for container %s of pod %s: %s", containerName, podName, err.Error())
										return
									}
									defer st.Close()
									lw := logwriter.New(func(lns []string) error {
										for _, l := range lns {
											lines <- fmt.Sprintf("LOG(%s/%s): %s", podName, containerName, l)
										}
										return nil
									})
									_, err = io.Copy(lw, st)
									if err != nil {
										lines <- fmt.Sprintf("error while reading/writing logs for container %s of pod %s: %s", containerName, podName, err.Error())
									}
									lines <- fmt.Sprintf("LOG(%s/%s): %s", podName, containerName, "END OF LOG")
								}()
							}
						}

					case watch.Deleted:
						pods.delete(podName)
						lines <- fmt.Sprintf("POD %s deleted", podName)
					}

				}
			}()

			deploymentWatch, err := deploymentsClient.Watch(context.Background(), v1.SingleObject(v1.ObjectMeta{Name: deploymentName}))
			if err != nil {
				return errors.Wrap(err, "while creating event watch")
			}

			defer deploymentWatch.Stop()

			go func() {
				for range deploymentWatch.ResultChan() {

				}
			}()

			for l := range lines {
				fmt.Println(l)
			}

			return nil
		},
	}
	a.RunAndExitOnError()
}

type podsByName struct {
	pods map[string]*corev1.Pod
	mu   *sync.Mutex
}

func newPodsByName() *podsByName {
	return &podsByName{
		pods: map[string]*corev1.Pod{},
		mu:   new(sync.Mutex),
	}
}

func (p *podsByName) contains(name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, found := p.pods[name]
	return found
}

func (p *podsByName) get(name string) *corev1.Pod {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pods[name]
}

func (p *podsByName) set(name string, pod *corev1.Pod) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pods[name] = pod
}

func (p *podsByName) delete(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.pods, name)
}
