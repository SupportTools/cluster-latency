package main

import (
	"context"
	"net/http"
	"os"

	"github.com/golang/glog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type App struct {
	kubeClient              *kubernetes.Clientset
	kubeNamespace           string
	kubePodName             string
	kubeNodeName            string
	kubePodZone             string
	myService               *v1.Service
	metricDownloadProbeSize *prometheus.GaugeVec
	metricDownloadDurations *prometheus.SummaryVec
	metricPingDurations     *prometheus.SummaryVec
}

func (a *App) testLoop() {
	for {
		podList, err := a.kubeClient.CoreV1().Pods(a.kubeNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: "app=cluster-latency",
		})
		if err != nil {
			glog.Errorln(err)
			panic(err)
		}
		glog.Infof("Pod list: %v", podList)
		for _, pod := range podList.Items {
			podName := pod.Name
			podIP := pod.Status.PodIP
			glog.Infof("Pod name: %s", podName)
			glog.Infof("Pod IP: %s", podIP)
		}
	}
}

func handlePing(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("pong"))
}

func main() {
	app := App{}
	app.Run()
}

func (a *App) Run() {
	App := App{
		metricDownloadProbeSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "download_probe_size",
				Help: "The size of the download probe",
			},
			[]string{"url"},
		),
		metricDownloadDurations: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "download_duration",
				Help: "The duration of the download",
			},
			[]string{"url"},
		),
		metricPingDurations: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "ping_duration",
				Help: "The duration of the ping",
			},
			[]string{"url"},
		),
	}

	glog.Info("Starting cluster-latency")

	// create the clientset
	config, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		glog.Errorln(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Errorln(err)
	}
	App.kubeClient = clientset

	// Getting namespace
	kubeNamespace := os.Getenv("KUBE_NAMESPACE")
	if kubeNamespace == "" {
		glog.Fatal("KUBE_NAMESPACE env var is required")
		panic("KUBE_NAMESPACE env var is required")
	}
	App.kubeNamespace = kubeNamespace
	glog.Infof("Kube namespace: %v", kubeNamespace)

	// Getting pod name
	kubePodName := os.Getenv("KUBE_POD_NAME")
	if kubePodName == "" {
		glog.Fatal("KUBE_POD_NAME env var is required")
		panic("KUBE_POD_NAME env var is required")
	}
	App.kubePodName = kubePodName
	glog.Infof("Kube pod name: %v", kubePodName)

	// Getting node name
	kubeNodeName := os.Getenv("KUBE_NODE_NAME")
	if kubeNodeName == "" {
		glog.Fatal("KUBE_NODE_NAME env var is required")
		panic("KUBE_NODE_NAME env var is required")
	}
	App.kubeNodeName = kubeNodeName
	glog.Infof("Kube node name: %v", kubeNodeName)

	// Getting pod zone
	nodeSpec, err := clientset.CoreV1().Nodes().Get(context.Background(), kubeNodeName, metav1.GetOptions{})
	if err != nil {
		glog.Errorln(err)
	}
	podZone := nodeSpec.Labels["failure-domain.beta.kubernetes.io/zone"]
	if podZone == "" {
		glog.Fatal("Pod zone is required")
		panic("Pod zone is required")
	}
	App.kubePodZone = podZone
	glog.Infof("Pod zone: %s", podZone)

	// get the service
	service, err := clientset.CoreV1().Services(kubeNamespace).Get(context.Background(), "cluster-latency", metav1.GetOptions{})
	if err != nil {
		glog.Errorln(err)
		panic(err)
	}
	serviceName := service.Name
	servicePort := service.Spec.Ports[0].Port
	glog.Infof("Service name: %s", serviceName)
	glog.Infof("Service port: %d", servicePort)

	// register prometheus metrics
	prometheus.MustRegister(App.metricDownloadProbeSize)
	prometheus.MustRegister(App.metricDownloadDurations)
	prometheus.MustRegister(App.metricPingDurations)

	// start web server
	glog.Infof("Starting web server on port %d", servicePort)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/ping", handlePing)

	glog.Info("Starting test loop")
	go a.testLoop()

}
