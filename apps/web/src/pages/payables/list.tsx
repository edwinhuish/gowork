import { SettlementList } from '@/pages/settlements/list'

export function PayableList() {
  return <SettlementList resource="payables" direction="payable" />
}
