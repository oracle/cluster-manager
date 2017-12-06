# Retrieving Wercker Clusters parameters

1. Create Cloud Credential. Use this information for the cluster-manager's OKE_CLOUD_AUTH_ID:
   1. Go to https://app.wercker.com/clusters/cloud-credentials
   1. Choose which Organization to use for the Cloud Credential
   1. Click "New Cloud Credential Button"
   1. Provide a Name
   1. Add all your BMC/OCI tenancy specific information (User OCID, Tenancy OCID). Get those information by logging in to OCI: https://console.us-phoenix-1.oraclecloud.com/ and then change Region to us-ashburn-1. Go to Identity to get User OCID. At the bottom of any page in OCI, you can get the Tenancy ID.
   1. For Key Fingerprint and API Private Key (PEM Format), follow the instruction in https://docs.us-phoenix-1.oraclecloud.com/Content/API/Concepts/apisigningkey.htm .
   1. Once all the required information is inputted, click create and take note of the cloud credential.
   1. The cloud credential can also be retrieved by going to Clusters->Cloud Credentials, choose the Organization and the name of your credential. From there the Cloud Auth Id can be retrieved.
1. To get the token that will be used for OKE_BEARER_TOKEN: 
   1. Click user icon on the top right corner of the app.wercker window.
   1. Choose Settings->Personal Tokens and generate a new token.
   1. Once the token is displayed, make sure to copy it as it won't be shown again.
1. To get the OKE_AUTH_GROUP
    1. Use this rest request ```curl -L app.wercker.com/api/v3/identity/lookup?q=<GROUP Name>```
    1. For example ```curl -L app.wercker.com/api/v3/identity/lookup?q=GAS{"id":"59cae3bd1521710100bc1449","name":"GAS"}```
    1. The id field value ("59cae3bd1521710100bc1449") reflects OKE_AUTH_GROUP
1. When logged in to the OCI console, also take note of the Compartment ID. Go to Identity->Compartment and choose the compartment where the instances will be created. Use this information in the cluster's cluster-manager.n6s.io/cluster.config annotation to specify which compartment will be used by the cluster.