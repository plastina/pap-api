"""Converte a Tabela de Medidas Referidas (IBGE, POF 2008-2009) para o CSV
embutido em internal/ibge.

Fonte (dominio publico, IBGE):
https://ftp.ibge.gov.br/Orcamentos_Familiares/Pesquisa_de_Orcamentos_Familiares_2008_2009/Tabela_de_Medidas_Referidas_para_os_Alimentos_Consumidos_no_Brasil/tabelamedidas_bd.zip

Uso (requer: pip install xlrd==2.0.1):
    python scripts/ibge_xls_to_csv.py tabelamedidas_bd.xls internal/ibge/medidas_pof2008.csv
"""
import csv
import sys

import xlrd


def as_int(v):
    return int(v) if isinstance(v, float) else v


def main(src, dst):
    sheet = xlrd.open_workbook(src).sheet_by_index(0)
    with open(dst, "w", newline="", encoding="utf-8") as f:
        out = csv.writer(f)
        out.writerow(["food_code", "food", "prep_code", "prep", "measure", "grams"])
        for r in range(sheet.nrows):
            v = sheet.row_values(r)
            # linhas de dados comecam pelo codigo numerico do alimento
            if not isinstance(v[0], float):
                continue
            out.writerow([as_int(v[0]), v[1].strip(), as_int(v[2]), v[3].strip(), v[5].strip(), v[8]])


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
