package com.jiansutech.yuqing.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import java.nio.file.Files

class StockResearchPdfDownloaderTest {
    @Test
    fun clearStockResearchPdfDirectoryDeletesDownloadedPdfFilesAndTempFiles() {
        val directory = Files.createTempDirectory("stock-research-pdfs").toFile()
        val nested = directory.resolve("nested")
        nested.mkdirs()
        val firstPdf = directory.resolve("first.pdf")
        val tempDownload = directory.resolve("second.pdf.download")
        val nestedPdf = nested.resolve("third.pdf")
        firstPdf.writeText("pdf")
        tempDownload.writeText("partial")
        nestedPdf.writeText("pdf")

        val deletedCount = clearStockResearchPdfDirectory(directory)

        assertEquals(3, deletedCount)
        assertFalse(firstPdf.exists())
        assertFalse(tempDownload.exists())
        assertFalse(nestedPdf.exists())
        assertFalse(directory.exists())
    }

    @Test
    fun clearStockResearchPdfDirectoryIgnoresMissingDirectory() {
        val directory = Files.createTempDirectory("stock-research-pdfs-missing").toFile()
        directory.delete()

        assertEquals(0, clearStockResearchPdfDirectory(directory))
    }
}
