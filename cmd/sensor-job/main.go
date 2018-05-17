/*
Copyright 2018 BlackRock, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/blackrock/axis/common"
	"github.com/blackrock/axis/job"
	"github.com/blackrock/axis/job/amqp"
	"github.com/blackrock/axis/job/artifact"
	"github.com/blackrock/axis/job/calendar"
	"github.com/blackrock/axis/job/kafka"
	"github.com/blackrock/axis/job/mqtt"
	"github.com/blackrock/axis/job/nats"
	"github.com/blackrock/axis/job/resource"
	"github.com/blackrock/axis/pkg/apis/sensor/v1alpha1"
	sensorclientset "github.com/blackrock/axis/pkg/client/clientset/versioned"
)

func main() {
	kubeConfig, _ := os.LookupEnv(common.EnvVarKubeConfig)

	config, err := common.GetClientConfig(kubeConfig)
	if err != nil {
		panic(err.Error())
	}

	sensorClientset := sensorclientset.NewForConfigOrDie(config)

	jobName, ok := os.LookupEnv(common.EnvVarJobName)
	if !ok {
		panic(fmt.Errorf("Unable to get job name from environment variable %s", common.EnvVarJobName))
	}
	namespace, ok := os.LookupEnv(common.EnvVarNamespace)
	if !ok {
		panic(fmt.Errorf("Unable to get job namespace from environment variable %s", common.EnvVarNamespace))
	}

	sensor, err := sensorClientset.AxisV1alpha1().Sensors(namespace).Get(common.ParseJobPrefix(jobName), metav1.GetOptions{})
	if err != nil {
		panic(err.Error())
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err.Error())
	}
	sensorLogger := logger.With(zap.String("sensor", sensor.Name))

	// find which signals to run
	registers, err := getSignalRegisters(sensor.Spec.Signals)
	if err != nil {
		panic(err.Error())
	}

	err = job.New(config, sensorClientset, sensorLogger).Run(sensor.DeepCopy(), registers)
	if err != nil {
		panic(err.Error())
	}
}

func getSignalRegisters(signals []v1alpha1.Signal) ([]func(*job.ExecutorSession), error) {
	var registerFuncs []func(*job.ExecutorSession)
	for _, signal := range signals {
		switch signal.GetType() {
		case v1alpha1.SignalTypeNats:
			registerFuncs = append(registerFuncs, nats.NATS)
		case v1alpha1.SignalTypeMQTT:
			registerFuncs = append(registerFuncs, mqtt.MQTT)
		case v1alpha1.SignalTypeAMQP:
			registerFuncs = append(registerFuncs, amqp.AMQP)
		case v1alpha1.SignalTypeKafka:
			registerFuncs = append(registerFuncs, kafka.Kafka)
		case v1alpha1.SignalTypeArtifact:
			registerFuncs = append(registerFuncs, artifact.Artifact)
			// for artifacts, need to find which stream to use
			switch signal.Artifact.NotificationStream.GetType() {
			case v1alpha1.StreamTypeNats:
				registerFuncs = append(registerFuncs, nats.NATS)
			case v1alpha1.StreamTypeMQTT:
				registerFuncs = append(registerFuncs, mqtt.MQTT)
			case v1alpha1.StreamTypeAMQP:
				registerFuncs = append(registerFuncs, amqp.AMQP)
			case v1alpha1.StreamTypeKafka:
				registerFuncs = append(registerFuncs, kafka.Kafka)
			default:
				return registerFuncs, fmt.Errorf("artifact signal does not define a notification output stream")
			}
		case v1alpha1.SignalTypeResource:
			registerFuncs = append(registerFuncs, resource.Resource)
		case v1alpha1.SignalTypeCalendar:
			registerFuncs = append(registerFuncs, calendar.Calendar)
		default:
			return registerFuncs, fmt.Errorf("%s signal type not supported", signal.GetType())
		}
	}
	return registerFuncs, nil
}