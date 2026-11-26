export interface ServiceField {
  key: string
  label: string
  type?: 'text' | 'date' | 'number' | 'select'
  required?: boolean
  placeholder?: string
  options?: Array<{ label: string; value: string }>
}

export interface DepartmentOperation {
  id: string
  title: string
  description: string
  method: 'GET' | 'POST' | 'PATCH'
  path: string
  minimumRole: 'clerk' | 'supervisor' | 'auditor'
  fields: ServiceField[]
  queryFields?: boolean
}

export interface DepartmentService {
  title: string
  description: string
  operations: DepartmentOperation[]
}

const sexOptions = [
  { label: 'Male', value: 'M' },
  { label: 'Female', value: 'F' },
  { label: 'Other', value: 'X' },
]

const citizenshipOptions = [
  { label: 'Citizen', value: 'CITIZEN' },
  { label: 'Permanent resident', value: 'PERMANENT_RESIDENT' },
  { label: 'Foreign', value: 'FOREIGN' },
  { label: 'Stateless', value: 'STATELESS' },
]

const maritalOptions = [
  { label: 'Single', value: 'SINGLE' },
  { label: 'Married', value: 'MARRIED' },
  { label: 'Divorced', value: 'DIVORCED' },
  { label: 'Widowed', value: 'WIDOWED' },
]

export const departmentServices: Record<string, DepartmentService> = {
  identity: {
    title: 'Identity services',
    description: 'Register and maintain protected identity records.',
    operations: [
      {
        id: 'person-lookup',
        title: 'Find a person',
        description: 'Look up an existing registry record using a national ID.',
        method: 'POST',
        path: '/v1/identity/persons/lookup',
        minimumRole: 'clerk',
        fields: [{ key: 'national_id', label: 'National ID', required: true, placeholder: 'Enter the national ID' }],
      },
      {
        id: 'person-register',
        title: 'Register a person',
        description: 'Create a new identity record.',
        method: 'POST',
        path: '/v1/identity/persons',
        minimumRole: 'supervisor',
        fields: [
          { key: 'national_id', label: 'National ID', required: true },
          { key: 'first_names', label: 'First names', required: true },
          { key: 'last_name', label: 'Last name', required: true },
          { key: 'date_of_birth', label: 'Date of birth', type: 'date', required: true },
          { key: 'sex', label: 'Sex', type: 'select', required: true, options: sexOptions },
          { key: 'citizenship_status', label: 'Citizenship status', type: 'select', required: true, options: citizenshipOptions },
          { key: 'marital_status', label: 'Marital status', type: 'select', options: maritalOptions },
        ],
      },
      {
        id: 'address-update',
        title: 'Update an address',
        description: 'Record a new current address for a registered person.',
        method: 'PATCH',
        path: '/v1/identity/persons/{person_id}/address',
        minimumRole: 'clerk',
        fields: [
          { key: 'person_id', label: 'Registry person ID', type: 'number', required: true },
          { key: 'line1', label: 'Address line', required: true },
          { key: 'city', label: 'City or town', required: true },
          { key: 'district', label: 'District', required: true },
          { key: 'postal_code', label: 'Postal code' },
          { key: 'valid_from', label: 'Valid from', type: 'date' },
        ],
      },
      {
        id: 'marriage-record',
        title: 'Record a marriage',
        description: 'Create a marriage record for two registered people.',
        method: 'POST',
        path: '/v1/identity/marriages',
        minimumRole: 'supervisor',
        fields: [
          { key: 'spouse_a_id', label: 'First spouse registry ID', type: 'number', required: true },
          { key: 'spouse_b_id', label: 'Second spouse registry ID', type: 'number', required: true },
          { key: 'married_on', label: 'Date of marriage', type: 'date', required: true },
          { key: 'certificate_no', label: 'Certificate number', required: true },
        ],
      },
    ],
  },
  health: {
    title: 'Health services',
    description: 'Verify identity and retrieve the health-service person projection.',
    operations: [
      {
        id: 'health-lookup',
        title: 'Look up a person',
        description: 'Retrieve a health-authorized identity projection.',
        method: 'POST',
        path: '/v1/health/persons/lookup',
        minimumRole: 'clerk',
        fields: [{ key: 'national_id', label: 'National ID', required: true }],
      },
    ],
  },
  grants: {
    title: 'Grants services',
    description: 'Verify a grant applicant against registry details.',
    operations: [
      {
        id: 'grants-verify',
        title: 'Verify an applicant',
        description: 'Check the applicant identity and last name.',
        method: 'POST',
        path: '/v1/grants/verify',
        minimumRole: 'clerk',
        fields: [
          { key: 'national_id', label: 'National ID', required: true },
          { key: 'last_name', label: 'Last name', required: true },
        ],
      },
    ],
  },
  police: {
    title: 'Police services',
    description: 'Verify an identity using registry details.',
    operations: [
      {
        id: 'police-verify',
        title: 'Verify an identity',
        description: 'Match a national ID and date of birth.',
        method: 'POST',
        path: '/v1/police/verify',
        minimumRole: 'clerk',
        fields: [
          { key: 'national_id', label: 'National ID', required: true },
          { key: 'date_of_birth', label: 'Date of birth', type: 'date', required: true },
        ],
      },
    ],
  },
  audit: {
    title: 'Audit services',
    description: 'Review access events recorded for your department.',
    operations: [
      {
        id: 'department-audit',
        title: 'Department audit log',
        description: 'Read recent department events. Dates are optional.',
        method: 'GET',
        path: '/v1/audit/my-department',
        minimumRole: 'auditor',
        queryFields: true,
        fields: [
          { key: 'from', label: 'From date', type: 'date' },
          { key: 'to', label: 'To date', type: 'date' },
          { key: 'limit', label: 'Maximum events', type: 'number', placeholder: '100' },
        ],
      },
    ],
  },
}

const roleRank: Record<string, number> = { clerk: 1, supervisor: 2, auditor: 3, admin: 4 }

export function availableOperations(department: string, role: string) {
  return (departmentServices[department]?.operations ?? [])
    .filter((operation) => (roleRank[role] ?? 0) >= roleRank[operation.minimumRole])
}
