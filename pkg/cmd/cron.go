package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/gorhill/cronexpr"
	"github.com/spf13/cobra"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

var (
	cronExample = `
	# view cronjobs
	%[1]s cron

	# view all cronjobs missed runs
	%[1]s ns --missed
`

	errNoContext = fmt.Errorf("no context is currently set, use %q to select a new one", "kubectl config use-context <context>")
)

type CronOptions struct {
	configFlags *genericclioptions.ConfigFlags

	rawConfig       api.Config
	chosenNamespace string
	numberOfMissed  int32

	args []string

	genericclioptions.IOStreams
}

func NewCronOptions(streams genericclioptions.IOStreams) *CronOptions {
	return &CronOptions{
		configFlags: genericclioptions.NewConfigFlags(true),

		IOStreams: streams,
	}
}

func NewCmdCron(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewCronOptions(streams)

	cmd := &cobra.Command{
		Use:          "cron [deploymebt-name] [flags]",
		Short:        "View cronjobs and missed runs",
		Example:      fmt.Sprintf(cronExample, "kubectl"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&o.chosenNamespace, "namespace", "n", "default", "Namespace for search. (default: \"default\")")
	cmd.Flags().Int32VarP(&o.numberOfMissed, "missed", "m", 1, "Number of missed runs")

	return cmd
}

// Validate ensures that all required arguments and flag values are provided
func (o *CronOptions) Validate() error {
	if len(o.args) > 1 {
		return fmt.Errorf("either one or no arguments are allowed")
	}

	return nil
}

func (o *CronOptions) CheckMissedCrons(cronName string, schedule string, lastScheduleTime time.Time, suspend bool) {
	if suspend == true {
		return
	}
	expr := cronexpr.MustParse(schedule)
	nextTime := expr.Next(lastScheduleTime)
	// fmt.Fprintf(o.Out, "Cron name: %s. Next cron time: %s\n", cronName, nextTime.String())
	dt := time.Now()
	if nextTime.Before(dt) {
		fmt.Fprintf(o.Out, "Cron name: %s. Cron missed it's run!. Current time: %s", cronName, dt.String())
	}
}

func (o *CronOptions) Run() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	fmt.Fprintf(o.Out, "Chosen namespace: %s\n", o.chosenNamespace)

	cronsListBatchV1Beta1, err := clientset.BatchV1beta1().CronJobs(o.chosenNamespace).List(context.TODO(), metav1.ListOptions{})
	cronsListV1, err := clientset.BatchV1().CronJobs(o.chosenNamespace).List(context.TODO(), metav1.ListOptions{})
	// fmt.Fprintf(o.Out, "Before cron range\n")
	for _, cron := range cronsListBatchV1Beta1.Items {
		o.CheckMissedCrons(cron.GetName(), cron.Spec.Schedule, cron.Status.LastScheduleTime.Time, *cron.Spec.Suspend)

	}
	for _, cron := range cronsListV1.Items {
		o.CheckMissedCrons(cron.GetName(), cron.Spec.Schedule, cron.Status.LastScheduleTime.Time, *cron.Spec.Suspend)
	}
	// fmt.Fprintf(o.Out, "After cron range\n")
	return nil
}
