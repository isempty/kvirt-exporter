package collector

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type VMCPUCollector struct {
	userUsage   *prometheus.GaugeVec
	systemUsage *prometheus.GaugeVec
	iowaitUsage *prometheus.GaugeVec
	tick        int64
}

func NewVMCPUCollector() (*VMCPUCollector, error) {
	tick, err := getClockTick()
	if err != nil {
		return nil, fmt.Errorf("failed to get CLK_TCK: %v", err)
	}

	return &VMCPUCollector{
		userUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vm_cpu_user_percent",
				Help: "User CPU usage percentage for VM",
			},
			[]string{"vm"},
		),
		systemUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vm_cpu_system_percent",
				Help: "System CPU usage percentage for VM",
			},
			[]string{"vm"},
		),
		iowaitUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vm_cpu_iowait_percent",
				Help: "Iowait CPU usage percentage for VM",
			},
			[]string{"vm"},
		),
		tick: tick,
	}, nil
}

func (c *VMCPUCollector) Describe(ch chan<- *prometheus.Desc) {
	c.userUsage.Describe(ch)
	c.systemUsage.Describe(ch)
	c.iowaitUsage.Describe(ch)
}

func (c *VMCPUCollector) Collect(ch chan<- prometheus.Metric) {
	vmList, err := getVMList()
	if err != nil {
		fmt.Printf("Error getting VM list: %v\n", err)
		return
	}

	for _, vm := range vmList {
		vcpuCount, err := getVCPUCount(vm)
		if err != nil || vcpuCount == 0 {
			fmt.Printf("Error getting vCPU count for %s: %v\n", vm, err)
			continue
		}

		pid, err := getQEMUPID(vm)
		if err != nil || pid == "" {
			fmt.Printf("Error getting QEMU PID for %s: %v\n", vm, err)
			continue
		}

		// 첫 번째 스냅샷
		utime1, stime1, err := getCPUStats(pid)
		if err != nil {
			fmt.Printf("Error getting CPU stats for %s: %v\n", vm, err)
			continue
		}
		iowait1, err := getIOWait()
		if err != nil {
			fmt.Printf("Error getting iowait for %s: %v\n", vm, err)
			continue
		}

		// 0.1초 대기
		time.Sleep(100 * time.Millisecond)

		// 두 번째 스냅샷
		utime2, stime2, err := getCPUStats(pid)
		if err != nil {
			fmt.Printf("Error getting second CPU stats for %s: %v\n", vm, err)
			continue
		}
		iowait2, err := getIOWait()
		if err != nil {
			fmt.Printf("Error getting second iowait for %s: %v\n", vm, err)
			continue
		}

		// 차이 계산
		utimeDiff := utime2 - utime1
		stimeDiff := stime2 - stime1
		iowaitDiff := iowait2 - iowait1

		// 총 가용 시간 (0.1초 * vCPU 수)
		totalInterval := float64(c.tick) / 10 * float64(vcpuCount)

		// 백분율 계산
		userPct := float64(utimeDiff) * 100 / totalInterval
		systemPct := float64(stimeDiff) * 100 / totalInterval
		iowaitPct := float64(iowaitDiff) * 100 / totalInterval

		// 음수 방지
		if userPct < 0 {
			userPct = 0
		}
		if systemPct < 0 {
			systemPct = 0
		}
		if iowaitPct < 0 {
			iowaitPct = 0
		}

		// 메트릭 설정
		c.userUsage.WithLabelValues(vm).Set(userPct)
		c.systemUsage.WithLabelValues(vm).Set(systemPct)
		c.iowaitUsage.WithLabelValues(vm).Set(iowaitPct)

		fmt.Printf("%s | user: %.2f%% | system: %.2f%% | iowait: %.2f%%\n", vm, userPct, systemPct, iowaitPct)
	}

	c.userUsage.Collect(ch)
	c.systemUsage.Collect(ch)
	c.iowaitUsage.Collect(ch)
}

func getClockTick() (int64, error) {
	tick, err := strconv.ParseInt(fmt.Sprintf("%d", os.Sysconf(os.SysconfName(_SC_CLK_TCK))), 10, 64)
	if err != nil {
		return 0, err
	}
	return tick, nil
}

func getVMList() ([]string, error) {
	cmd := exec.Command("virsh", "list", "--name")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	vms := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, vm := range vms {
		if vm != "" {
			result = append(result, vm)
		}
	}
	return result, nil
}

func getVCPUCount(vm string) (int, error) {
	cmd := exec.Command("virsh", "dominfo", vm)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "CPU(s)") {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				return strconv.Atoi(parts[1])
			}
		}
	}
	return 0, nil
}

func getQEMUPID(vm string) (string, error) {
	cmd := exec.Command("ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.Contains(line, "qemu-system") && strings.Contains(line, vm) {
			parts := strings.Fields(line)
			if len(parts) > 1 {
				return parts[1], nil
			}
		}
	}
	return "", nil
}

func getCPUStats(pid string) (int64, int64, error) {
	var totalUtime, totalStime int64
	tasks, err := filepath.Glob(fmt.Sprintf("/proc/%s/task/*/stat", pid))
	if err != nil {
		return 0, 0, err
	}
	for _, task := range tasks {
		data, err := os.ReadFile(task)
		if err != nil {
			continue
		}
		parts := strings.Fields(string(data))
		if len(parts) < 15 {
			continue
		}
		utime, _ := strconv.ParseInt(parts[13], 10, 64)
		stime, _ := strconv.ParseInt(parts[14], 10, 64)
		totalUtime += utime
		totalStime += stime
	}
	return totalUtime, totalStime, nil
}

func getIOWait() (int64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "cpu ") {
			parts := strings.Fields(line)
			if len(parts) > 5 {
				return strconv.ParseInt(parts[5], 10, 64)
			}
		}
	}
	return 0, nil
}
