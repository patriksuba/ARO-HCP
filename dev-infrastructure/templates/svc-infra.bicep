@description('The name of the service keyvault')
param serviceKeyVaultName string

@description('The name of the resourcegroup for the service keyvault')
param serviceKeyVaultResourceGroup string = resourceGroup().name

@description('The location of the resourcegroup for the service keyvault')
param serviceKeyVaultLocation string = resourceGroup().location

@description('Soft delete setting for service keyvault')
param serviceKeyVaultSoftDelete bool = true

@description('If true, make the service keyvault private and only accessible by the svc cluster via private link.')
param serviceKeyVaultPrivate bool = true

@description('KV certificate officer principal ID')
param kvCertOfficerPrincipalId string

@description('MSI that will be used during pipeline runs')
param aroDevopsMsiId string

@description('Set to true to prevent resources from being pruned after 48 hours')
param persist bool = false

// Tags the resource group
resource resourcegroupTags 'Microsoft.Resources/tags@2024-03-01' = {
  name: 'default'
  scope: resourceGroup()
  properties: {
    tags: {
      persist: toLower(string(persist))
    }
  }
}

// Reader role
// https://www.azadvertizer.net/azrolesadvertizer/acdd72a7-3385-48ef-bd42-f606fba81ae7.html
var readerRoleId = subscriptionResourceId(
  'Microsoft.Authorization/roleDefinitions',
  'acdd72a7-3385-48ef-bd42-f606fba81ae7'
)

// service deployments running as the aroDevopsMsi need to lookup metadata about all kinds
// of resources, e.g. AKS metadata, database metadata, MI metadata, etc.
resource aroDevopsMSIReader 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(resourceGroup().id, aroDevopsMsiId, readerRoleId)
  properties: {
    principalId: reference(aroDevopsMsiId, '2023-01-31').principalId
    principalType: 'ServicePrincipal'
    roleDefinitionId: readerRoleId
  }
}

//
//   K E Y V A U L T S
//

module serviceKeyVault '../modules/keyvault/keyvault.bicep' = {
  name: 'svc-kv'
  scope: resourceGroup(serviceKeyVaultResourceGroup)
  params: {
    location: serviceKeyVaultLocation
    keyVaultName: serviceKeyVaultName
    private: serviceKeyVaultPrivate
    enableSoftDelete: serviceKeyVaultSoftDelete
    purpose: 'service'
  }
}

module serviceKeyVaultCertOfficer '../modules/keyvault/keyvault-secret-access.bicep' = {
  name: 'svc-kv-cert-officer'
  scope: resourceGroup(serviceKeyVaultResourceGroup)
  params: {
    keyVaultName: serviceKeyVaultName
    roleName: 'Key Vault Certificates Officer'
    managedIdentityPrincipalId: kvCertOfficerPrincipalId
  }
  dependsOn: [
    serviceKeyVault
  ]
}

module serviceKeyVaultSecretsOfficer '../modules/keyvault/keyvault-secret-access.bicep' = {
  name: 'svc-kv-secret-officer'
  scope: resourceGroup(serviceKeyVaultResourceGroup)
  params: {
    keyVaultName: serviceKeyVaultName
    roleName: 'Key Vault Secrets Officer'
    managedIdentityPrincipalId: kvCertOfficerPrincipalId
  }
  dependsOn: [
    serviceKeyVault
  ]
}

module serviceKeyVaultDevopsSecretsOfficer '../modules/keyvault/keyvault-secret-access.bicep' = {
  name: 'svc-kv-devops-secret-officer'
  scope: resourceGroup(serviceKeyVaultResourceGroup)
  params: {
    keyVaultName: serviceKeyVaultName
    roleName: 'Key Vault Secrets Officer'
    managedIdentityPrincipalId: reference(aroDevopsMsiId, '2023-01-31').principalId
  }
  dependsOn: [
    serviceKeyVault
  ]
}

output svcKeyVaultName string = serviceKeyVault.outputs.kvName
output svcKeyVaultUrl string = serviceKeyVault.outputs.kvUrl
