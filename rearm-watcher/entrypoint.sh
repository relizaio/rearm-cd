#!/bin/bash

if [ -z "$REARM_URI" ]
then
    echo "REARM_URI must be defined"
    exit 1
fi

echo "REARM_URI is set to $REARM_URI"

date +"%s" > /resources/last_sent
while [ true ]
do
    if [ "$NAMESPACE" == "allnamespaces" ]
    then
        readarray -t NAMESPACES < <(kubectl get ns -o custom-columns=NAME:.metadata.name --no-headers)
    else
        IFS="," read -ra NAMESPACES <<< "$NAMESPACE"
    fi
    for ns in "${NAMESPACES[@]}"; do
        if [ "$ns" != "NAME" ]
        then
            kubectl get po -n $ns -o json | jq "[.items[] | {namespace:.metadata.namespace, labels:.metadata.labels, annotations:.metadata.annotations, pod:.metadata.name, status:.status.containerStatuses[]}]" > /resources/ns/${ns}_images_new
            # initialize old file if it doesn't exist
            if [ ! -f /resources/ns/${ns}_images ]; then
                echo "[]" > /resources/ns/${ns}_images
            fi
            difflines=$(diff /resources/ns/${ns}_images_new /resources/ns/${ns}_images | wc -l)
            if [ $difflines -gt 0 ]
            then
                echo "$(date) - change in images detected for namespace $ns - shipping to ReARM"
                rearm devops instdata -u $REARM_URI -i $REARM_API_ID -k $REARM_API_KEY --sender $SENDER_ID$ns --namespace $ns --imagestyle k8s --imagefile /resources/ns/${ns}_images_new
                date +"%s" > /resources/last_sent
            fi
            mv /resources/ns/${ns}_images_new /resources/ns/${ns}_images
        fi
        sleep 5
    done
    sleep 30
done