#!/bin/bash
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.103 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.104 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.105 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.106 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.107 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.108 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.109 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"
ssh -i /home/lichenrui/ruc_500 centos@10.77.70.120 "sudo rm -rf /home/centos/llm/workflow/volumes/codes/cryptService/cryptService"

scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.103:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.104:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.104:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.105:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.106:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.107:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.108:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.109:/home/centos/llm/workflow/volumes/codes/cryptService/
scp -i /home/lichenrui/ruc_500 -r cryptService centos@10.77.70.120:/home/centos/llm/workflow/volumes/codes/cryptService/



