package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"

	"github.com/jedib0t/go-pretty/v6/table"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	cronExample = `
	# view cronjobs in a namespace with a json format
	%[1]s --namespace mynamespace

	# view all cronjobs missed runs
	%[1]s --missed

	# view all cronjobs in a table format
	%[1]s --format table
`
)

type CronOptions struct {
	configFlags     *genericclioptions.ConfigFlags
	chosenNamespace string
	format          string
	missed          bool
	debug           bool
	args            []string
	genericclioptions.IOStreams
}

type Output struct {
	Schedule         string
	LastScheduleTime string
	NextScheduleTime string
	Suspended        bool
	Missed           string
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
		Use:          "[flags]",
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
	cmd.Flags().StringVarP(&o.format, "format", "f", "table", "The format of the output. Possible choices: table, json")
	cmd.Flags().BoolVarP(&o.missed, "missed", "m", false, "Show only crons with missed runs")
	cmd.Flags().BoolVarP(&o.debug, "debug", "d", false, "Debug")

	return cmd
}

// Validate ensures that all required arguments and flag values are provided
func (o *CronOptions) Validate() error {
	if len(o.args) > 1 {
		return fmt.Errorf("either one or no arguments are allowed")
	}

	return nil
}

func (o *CronOptions) FillCronStatus(cronName string, schedule string, lastScheduleTimeFormatted string, suspend bool, output map[string]Output) {
	if o.missed && suspend {
		return
	}

	utcLocation, _ := time.LoadLocation("UTC")
	currentLocation := time.Now().Location()
	nextRunFormatted := ""
	missedRunFormatted := ""
	missedRun := false
	if !suspend {
		cronParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		parsedCron, _ := cronParser.Parse(schedule)
		lastScheduleTime, _ := time.Parse(time.RFC3339, lastScheduleTimeFormatted)
		lastScheduleTime = lastScheduleTime.In(utcLocation)
		nextRun := parsedCron.Next(lastScheduleTime)
		nextRunFormatted = nextRun.In(currentLocation).Format(time.RFC3339)
		dt := time.Now()
		missedRun = nextRun.Before(dt)
		if missedRun {
			missedRunFormatted = "Cron missed it's run!"
		}
	}

	cronOutput := Output{
		Schedule:         schedule,
		LastScheduleTime: lastScheduleTimeFormatted,
		NextScheduleTime: nextRunFormatted,
		Suspended:        suspend,
		Missed:           missedRunFormatted,
	}

	if o.missed {
		if missedRun {
			output[cronName] = cronOutput
		}
	} else {
		output[cronName] = cronOutput
	}
}

func (o *CronOptions) PrintAsJson(output map[string]Output) error {
	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return err
	}

	fmt.Fprintf(o.Out, "%s", jsonOutput)
	return nil
}

func (o *CronOptions) PrintAsTable(output map[string]Output) error {
	t := table.NewWriter()
	t.SetOutputMirror(o.Out)
	t.AppendHeader(table.Row{"#", "Cron Name", "Cron Schedule", "Last Schedule Time", "Next Schedule Time", "Suspended", "Missed"})

	keys := make([]string, 0)
	for k, _ := range output {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	index := 0
	for _, k := range keys {
		index++
		v := output[k]
		t.AppendRow(table.Row{index, k, v.Schedule, v.LastScheduleTime, v.NextScheduleTime, v.Suspended, v.Missed})
		t.AppendSeparator()
	}

	t.Render()
	return nil
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

	if o.debug {
		fmt.Fprintf(o.Out, "Chosen namespace: %s\n", o.chosenNamespace)
	}

	cronsListBatchV1Beta1, _ := clientset.BatchV1beta1().CronJobs(o.chosenNamespace).List(context.TODO(), metav1.ListOptions{})

	cronsListBatchV1, _ := clientset.BatchV1().CronJobs(o.chosenNamespace).List(context.TODO(), metav1.ListOptions{})

	output := make(map[string]Output)

	if o.debug {
		fmt.Fprintf(o.Out, "Before cron range\n")
	}
	for _, cron := range cronsListBatchV1Beta1.Items {
		if o.debug {
			fmt.Fprintf(o.Out, "BatchV1Beta1 Cron: %v. Suspend: %v. Scheduled: %v\n", cron.GetName(), *cron.Spec.Suspend, cron.Spec.Schedule)
		}
		lastScheduleTimeFormatted := ""
		if !*cron.Spec.Suspend {
			lastScheduleTimeFormatted = cron.Status.LastScheduleTime.Time.Format(time.RFC3339)
		}
		o.FillCronStatus(cron.GetName(), cron.Spec.Schedule, lastScheduleTimeFormatted, *cron.Spec.Suspend, output)

	}
	for _, cron := range cronsListBatchV1.Items {
		if o.debug {
			fmt.Fprintf(o.Out, "BatchV1 Cron: %v. Suspend: %v. Scheduled: %v\n", cron.GetName(), *cron.Spec.Suspend, cron.Spec.Schedule)
		}
		lastScheduleTimeFormatted := ""
		if !*cron.Spec.Suspend {
			lastScheduleTimeFormatted = cron.Status.LastScheduleTime.Time.Format(time.RFC3339)
		}
		o.FillCronStatus(cron.GetName(), cron.Spec.Schedule, lastScheduleTimeFormatted, *cron.Spec.Suspend, output)
	}
	if o.debug {
		fmt.Fprintf(o.Out, "After cron range\n")
	}

	if o.format == "table" {
		o.PrintAsTable(output)
	} else {
		o.PrintAsJson(output)
	}

	return nil
}
