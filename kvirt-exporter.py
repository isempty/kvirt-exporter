#!/usr/bin/env python3

import subprocess
import time
import re
from prometheus_client import start_http_server, Gauge
import os

# Prometheus 메트릭 정의
user_usage = Gauge('vm_cpu_user_percent', 'User CPU usage percentage for VM', ['vm'])
system_usage = Gauge('vm_cpu_system_percent', 'System CPU usage percentage for VM', ['vm'])
iowait_usage = Gauge('vm_cpu_iowait_percent', 'Iowait CPU usage percentage for VM', ['vm'])

def get_vm_list():
    """실행 중인 VM 목록 가져오기"""
    try:
        result = subprocess.run(['virsh', 'list', '--name'], capture_output=True, text=True, check=True)
        return [vm for vm in result.stdout.splitlines() if vm.strip()]
    except subprocess.CalledProcessError:
        print("Error: Failed to get VM list")
        return []

def get_vcpu_count(vm):
    """VM의 vCPU 수 가져오기"""
    try:
        result = subprocess.run(['virsh', 'dominfo', vm], capture_output=True, text=True, check=True)
        for line in result.stdout.splitlines():
            if 'CPU(s)' in line:
                return int(line.split(':')[1].strip())
        return 0
    except (subprocess.CalledProcessError, ValueError):
        print(f"Error: Failed to get vCPU count for {vm}")
        return 0

def get_qemu_pid(vm):
    """VM의 QEMU 프로세스 PID 가져오기"""
    try:
        result = subprocess.run(['ps', 'aux'], capture_output=True, text=True, check=True)
        for line in result.stdout.splitlines():
            if f'qemu-system' in line and vm in line:
                return line.split()[1]
        return None
    except subprocess.CalledProcessError:
        print(f"Error: Failed to get QEMU PID for {vm}")
        return None

def get_cpu_stats(pid):
    """PID의 모든 스레드에서 utime, stime 합계 가져오기"""
    total_utime = 0
    total_stime = 0
    try:
        for task in os.listdir(f'/proc/{pid}/task'):
            stat_file = f'/proc/{pid}/task/{task}/stat'
            if os.path.exists(stat_file):
                with open(stat_file, 'r') as f:
                    stats = f.read().split()
                    total_utime += int(stats[13])  # utime
                    total_stime += int(stats[14])  # stime
        return total_utime, total_stime
    except (OSError, ValueError):
        return 0, 0

def get_iowait():
    """시스템 iowait 값 가져오기"""
    try:
        with open('/proc/stat', 'r') as f:
            for line in f:
                if line.startswith('cpu '):
                    return int(line.split()[5])  # iowait
        return 0
    except (OSError, ValueError):
        return 0

def collect_metrics():
    """VM 메트릭 수집"""
    tick = os.sysconf(os.sysconf_names['SC_CLK_TCK'])
    vm_list = get_vm_list()

    if not vm_list:
        print("No running VMs found.")
        return

    for vm in vm_list:
        vcpu_count = get_vcpu_count(vm)
        if vcpu_count == 0:
            print(f"{vm}: No vCPUs or VM inactive")
            continue

        pid = get_qemu_pid(vm)
        if not pid:
            print(f"{vm}: QEMU process not found")
            continue

        # 첫 번째 스냅샷
        utime1, stime1 = get_cpu_stats(pid)
        iowait1 = get_iowait()

        # 0.1초 대기
        time.sleep(0.1)

        # 두 번째 스냅샷
        utime2, stime2 = get_cpu_stats(pid)
        iowait2 = get_iowait()

        # 차이 계산
        utime_diff = utime2 - utime1
        stime_diff = stime2 - stime1
        iowait_diff = iowait2 - iowait1

        # 총 가용 시간 (0.1초 * vCPU 수)
        total_interval = (tick // 10) * vcpu_count

        # 백분율 계산
        user_pct = (utime_diff * 100) / total_interval if total_interval > 0 else 0
        system_pct = (stime_diff * 100) / total_interval if total_interval > 0 else 0
        iowait_pct = (iowait_diff * 100) / total_interval if total_interval > 0 else 0

        # 음수 방지
        user_pct = max(0, user_pct)
        system_pct = max(0, system_pct)
        iowait_pct = max(0, iowait_pct)

        # Prometheus 메트릭 설정
        user_usage.labels(vm=vm).set(round(user_pct, 2))
        system_usage.labels(vm=vm).set(round(system_pct, 2))
        iowait_usage.labels(vm=vm).set(round(iowait_pct, 2))

        print(f"{vm} | user: {user_pct:.2f}% | system: {system_pct:.2f}% | iowait: {iowait_pct:.2f}%")

def main():
    # HTTP 서버 시작 (포트 8099)
    start_http_server(8099)
    print("Prometheus exporter started on port 8099")

    while True:
        try:
            collect_metrics()
        except Exception as e:
            print(f"Error collecting metrics: {e}")
        time.sleep(5)  # 5초마다 메트릭 수집

if __name__ == '__main__':
    main()
