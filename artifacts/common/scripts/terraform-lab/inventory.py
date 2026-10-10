#!/usr/bin/env python3
"""Преобразовать только Terraform output lab в закрытый Ansible inventory."""

import argparse
import ipaddress
import json
import re
import sys


def inventory(lab, user):
    if not re.fullmatch(r"[a-z][a-z0-9_-]{0,30}", user) or user == "root":
        raise ValueError("Неподдерживаемый SSH-пользователь.")
    if not isinstance(lab, dict):
        raise ValueError("Ожидается объект terraform output -json lab.")
    for field in ("folder_id", "network_id", "gateway_id", "route_table_id"):
        if not re.fullmatch(r"[a-z0-9]{20}", lab.get(field, "")):
            raise ValueError("Отсутствует или неверен ID ресурса.")
    if lab.get("egress_mode") not in ("bootstrap", "restricted"):
        raise ValueError("Неверный egress_mode.")
    for role in ("app", "proxy"):
        node = lab[role]
        for field in ("id", "boot_disk_id"):
            if not re.fullmatch(r"[a-z0-9]{20}", node[field]):
                raise ValueError("Неверный ID VM или диска.")
        address = ipaddress.IPv4Address(node["private_ip"])
        if not address.is_private or address.is_loopback or address.is_link_local:
            raise ValueError("VM должна иметь частный IPv4.")
    public = ipaddress.IPv4Address(lab["proxy"]["public_ip"])
    if not public.is_global or lab["app"].get("public_ip"):
        raise ValueError("Public IPv4 допускается только у proxy.")
    app_ip = str(ipaddress.IPv4Address(lab["app"]["private_ip"]))
    proxy_ip = str(ipaddress.IPv4Address(lab["proxy"]["private_ip"]))
    if app_ip == proxy_ip:
        raise ValueError("Частные адреса VM должны отличаться.")
    ssh_args = "-o StrictHostKeyChecking=yes -o ForwardAgent=no -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=2"
    return {
        "all": {
            "vars": {"ansible_user": user, "ansible_python_interpreter": "/usr/bin/python3"},
            "children": {
                "app": {
                    "hosts": {
                        "app-01": {
                            "ansible_host": app_ip,
                            "ansible_ssh_common_args": f"-o ProxyJump={user}@{public} {ssh_args}",
                            "application_bind_ip": app_ip,
                            "application_proxy_url": f"http://{proxy_ip}",
                            "application_dir": "/opt/devops-practice-ansible-07",
                            "application_image": "devops-practice:lab-07-v1",
                            "application_tool_image": "devops-practice:lab-07-v1",
                            "application_postgres_user": "devops",
                            "application_postgres_db": "devops_practice",
                        }
                    }
                },
                "proxy": {
                    "hosts": {
                        "proxy-01": {
                            "ansible_host": str(public),
                            "ansible_ssh_common_args": ssh_args,
                            "nginx_upstream_ip": app_ip,
                            "nginx_response_label": "terraform-lab-08",
                        }
                    }
                },
            },
        }
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--user", required=True)
    args = parser.parse_args()
    try:
        result = inventory(json.load(sys.stdin), args.user)
    except (ValueError, KeyError, TypeError):
        print("Неверный lab output или SSH user; содержимое не выводится.", file=sys.stderr)
        return 2
    json.dump(result, sys.stdout, ensure_ascii=False, indent=2)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
