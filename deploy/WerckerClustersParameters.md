# Setting up Wercker Clusters parameters to work with Cluster Manager

If you are using [Wercker Clusters](http://devcenter.wercker.com/docs/getting-started-with-wercker-clusters#creatingcluster), you need to set the *OKE_CLOUD_AUTH_ID*,  *OKE_BEARER_TOKEN*,  *OKE_AUTH_GROUP*,  and Compartment ID parameters to work with Cluster Manager. If you do not have these parameters, then follow this document to create them.  

1. When you use Wercker Clusters to create a new Kubernetes cluster, you specify Cloud Credentials to specify where you want to create the cluster. If you already have *Cloud Auth ID* and OKE_AUTH_GROUP in the **Clusters > Cloud Credentials** page in the Wercker cluster, you can use those credentials. If you have to create those IDs, then follow these steps:
    <ol type="a">
    <li>Go to https://app.wercker.com/clusters/cloud-credentials and select the Organization.</li>
    <li>Click **New Cloud Credential Button**. </li>
    <li>Enter a name.</li>
    <li>Enter all your Oracle Cloud Infrastructure (OCI) tenancy specific information (User OCID, Tenancy OCID). You can get those information by logging in to [OCI](https://console.us-phoenix-1.oraclecloud.com/) and then by changing the Region, for example, change the region to *us-ashburn-1*. Navigate to Identity and note down the User OCID and also copy the Tenancy ID, which you will find at the bottom of any page in OCI.</li>
    <li>For **Key Fingerprint** and **API Private Key (PEM Format)**, follow the instructions in https://docs.us-phoenix-1.oraclecloud.com/Content/API/Concepts/apisigningkey.htm.</li>
    <li> Click **Create** and note down the Cloud Auth ID. The Cloud_Auth_ID is displayed in the Cloud Credentials page.</li>
    <li>In the Cloud Credentials page, note down the OKE_AUTH_GROUP, which is the ID displayed next to the Organization drop down list. 
    </ol>
1. Create a new Wercker authentication token if you do not have one, which will be used for *OKE_BEARER_TOKEN*. See [how to get a Wercker authentication token] (http://devcenter.wercker.com/docs/getting-started-with-wercker-releases#gettingtoken) for more details.
1. You also need a Compartment ID to specify which compartment the cluster will use. When you log in to the OCI console, go to Identity->Compartment and choose the compartment where the instances will be created. 